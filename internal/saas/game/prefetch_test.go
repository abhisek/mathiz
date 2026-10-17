package game

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/store"
)

// blockGen blocks every Generate until the test releases it (one token per
// call) or the call's context ends. It records each call's context so a
// test can see whether a generation was cancelled.
type blockGen struct {
	mu      sync.Mutex
	calls   int
	ctxs    []context.Context
	started chan int
	release chan struct{}
}

func newBlockGen() *blockGen {
	return &blockGen{started: make(chan int, 16), release: make(chan struct{}, 16)}
}

func (g *blockGen) Generate(ctx context.Context, input problemgen.GenerateInput) (*problemgen.Question, error) {
	g.mu.Lock()
	g.calls++
	n := g.calls
	g.ctxs = append(g.ctxs, ctx)
	g.mu.Unlock()
	g.started <- n
	select {
	case <-g.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &problemgen.Question{
		Text:       fmt.Sprintf("What is 2 + 2? (b%d)", n),
		Format:     problemgen.FormatNumeric,
		Answer:     "4",
		AnswerType: problemgen.AnswerTypeInteger,
		SkillID:    input.Skill.ID,
		Tier:       input.Tier,
	}, nil
}

func (g *blockGen) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func (g *blockGen) ctx(i int) context.Context {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ctxs[i]
}

func (g *blockGen) waitStarted(t *testing.T) int {
	t.Helper()
	select {
	case n := <-g.started:
		return n
	case <-time.After(2 * time.Second):
		t.Fatal("generation never started")
		return 0
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestStartAndAnswerWarmTheNextQuestion(t *testing.T) {
	gen := &fakeGenerator{}
	m := newTestManager(t, gen)
	ctx := context.Background()
	exp, err := m.Start(ctx, "child-1", rootSkillID(t))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	// Q1 generates during "Setting sail…", before the client asks.
	waitFor(t, "Q1 warm", func() bool { return gen.callCount() == 1 })
	q, err := m.Question(ctx, "child-1", exp.ID)
	if err != nil {
		t.Fatalf("question 1: %v", err)
	}
	if q.Source != SourcePrefetchHit && q.Source != SourcePrefetchJoin {
		t.Errorf("Q1 source = %q, want a prefetch source", q.Source)
	}
	if _, err := m.Answer(ctx, "child-1", exp.ID, "4"); err != nil {
		t.Fatalf("answer: %v", err)
	}
	// Q2 generates while the kid reads the feedback.
	waitFor(t, "Q2 warm", func() bool { return gen.callCount() == 2 })
	q2, err := m.Question(ctx, "child-1", exp.ID)
	if err != nil {
		t.Fatalf("question 2: %v", err)
	}
	if q2.Text == q.Text || gen.callCount() != 2 {
		t.Errorf("Q2 = %q after %d calls; want a new question and no extra call", q2.Text, gen.callCount())
	}
}

func TestDisablePrefetchStillServesQuestions(t *testing.T) {
	gen := &fakeGenerator{}
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m := NewManager(Config{
		Store:           st,
		Toolset:         func(context.Context, store.EventRepo) (*Toolset, error) { return &Toolset{Generator: gen}, nil },
		DisablePrefetch: true,
	})
	ctx := context.Background()
	exp, err := m.Start(ctx, "child-1", rootSkillID(t))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if gen.callCount() != 0 {
		t.Fatalf("off switch still warmed: %d calls", gen.callCount())
	}
	q, err := m.Question(ctx, "child-1", exp.ID)
	if err != nil || q.Source != SourceFresh {
		t.Fatalf("question = %+v, %v; want a fresh one", q, err)
	}
}

func TestEndDoesNotWaitForGeneration(t *testing.T) {
	gen := newBlockGen()
	m := newTestManager(t, gen)
	ctx := context.Background()
	exp, err := m.Start(ctx, "child-1", rootSkillID(t))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	gen.waitStarted(t) // the Q1 warm hangs

	qErr := make(chan error, 1)
	go func() {
		_, err := m.Question(ctx, "child-1", exp.ID)
		qErr <- err
	}()
	time.Sleep(20 * time.Millisecond) // let Question join the hung warm

	// "Sail home" answers at once, and the hung generation is cancelled
	// before End returns (the play slot is freed after it).
	ended := make(chan struct{})
	go func() {
		if _, err := m.End(ctx, "child-1", exp.ID); err != nil {
			t.Errorf("end: %v", err)
		}
		close(ended)
	}()
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("End waited on the LLM")
	}
	if gen.ctx(0).Err() == nil {
		t.Error("the warm outlived the expedition")
	}
	if err := <-qErr; !errors.Is(err, ErrExpeditionOver) && !errors.Is(err, ErrNoExpedition) {
		t.Errorf("waiting Question = %v, want the expedition over", err)
	}
	// The slot is free: a new expedition starts.
	if _, err := m.Start(ctx, "child-1", rootSkillID(t)); err != nil {
		t.Errorf("start after end: %v", err)
	}
}

func TestDoubleTapSharesOneGeneration(t *testing.T) {
	gen := newBlockGen()
	m := newTestManager(t, gen)
	ctx := context.Background()
	exp, err := m.Start(ctx, "child-1", rootSkillID(t))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	gen.waitStarted(t)

	var wg sync.WaitGroup
	views := make([]*QuestionView, 2)
	for i := range views {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := m.Question(ctx, "child-1", exp.ID)
			if err != nil {
				t.Errorf("tap %d: %v", i, err)
			}
			views[i] = v
		}()
	}
	time.Sleep(20 * time.Millisecond)
	gen.release <- struct{}{}
	wg.Wait()
	if views[0] == nil || views[1] == nil || views[0].Text != views[1].Text || views[0].Index != 1 || views[1].Index != 1 {
		t.Fatalf("taps got %+v and %+v; want the same first question", views[0], views[1])
	}
	if gen.callCount() != 1 {
		t.Errorf("generator calls = %d, want 1", gen.callCount())
	}
}

func TestAnswerAndHintDuringGeneration(t *testing.T) {
	gen := newBlockGen()
	m := newTestManager(t, gen)
	ctx := context.Background()
	exp, err := m.Start(ctx, "child-1", rootSkillID(t))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	gen.waitStarted(t)
	go func() { _, _ = m.Question(ctx, "child-1", exp.ID) }()
	time.Sleep(20 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := m.Answer(ctx, "child-1", exp.ID, "4"); !errors.Is(err, ErrNoQuestion) {
			t.Errorf("answer during generation = %v, want ErrNoQuestion", err)
		}
		if _, err := m.Hint(ctx, "child-1", exp.ID); err == nil {
			t.Error("hint during generation succeeded")
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Answer/Hint blocked on the LLM")
	}
	gen.release <- struct{}{}
}

func TestLeftClientDoesNotTripBreaker(t *testing.T) {
	gen := newBlockGen()
	m := newTestManager(t, gen)
	ctx := context.Background()
	exp, err := m.Start(ctx, "child-1", rootSkillID(t))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	gen.waitStarted(t)
	// Three requests whose kid left before the question came.
	for i := 0; i < maxGenFailures; i++ {
		gone, cancel := context.WithTimeout(ctx, 5*time.Millisecond)
		_, err := m.Question(gone, "child-1", exp.ID)
		cancel()
		if !errors.Is(err, ErrGeneration) {
			t.Fatalf("request %d = %v, want ErrGeneration", i, err)
		}
	}
	// The expedition is alive, and the generation that kept running serves
	// the next request.
	gen.release <- struct{}{}
	q, err := m.Question(ctx, "child-1", exp.ID)
	if err != nil || q.Index != 1 {
		t.Fatalf("question after the kid came back = %+v, %v", q, err)
	}
	if gen.callCount() != 1 {
		t.Errorf("generator calls = %d, want 1", gen.callCount())
	}
}

func TestBreakerChargesOnce(t *testing.T) {
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	var charges int
	m := NewManager(Config{
		Store: st,
		Toolset: func(context.Context, store.EventRepo) (*Toolset, error) {
			return &Toolset{Generator: &fakeGenerator{fail: true}}, nil
		},
		Charge: func(context.Context, string, string) error {
			charges++
			return nil
		},
	})
	ctx := context.Background()
	exp, err := m.Start(ctx, "child-1", rootSkillID(t))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	for i := 0; i < maxGenFailures+2; i++ {
		_, _ = m.Question(ctx, "child-1", exp.ID)
	}
	if _, err := m.End(ctx, "child-1", exp.ID); err != nil {
		t.Fatalf("end: %v", err)
	}
	if charges != 1 {
		t.Errorf("charges = %d, want 1: failures and retries never debit", charges)
	}
}

func TestQuestExpeditionsNeverWarm(t *testing.T) {
	m := newQuestTestManager(t, newFakeQuestSource("", 3))
	ctx := context.Background()
	exp, err := m.StartQuest(ctx, "child-1", "quest-1")
	if err != nil {
		t.Fatalf("start quest: %v", err)
	}
	live, err := m.lookup("child-1", exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if live.prefetch != nil {
		t.Fatal("a quest expedition has a prefetcher: a discarded warm would skip a parent's question")
	}
	// Each question is the next authored one, in order.
	for i := 1; i <= 2; i++ {
		q, err := m.Question(ctx, "child-1", exp.ID)
		if err != nil || q.Index != i || q.Source != SourceQuest {
			t.Fatalf("quest question %d = %+v, %v", i, q, err)
		}
		if _, err := m.Answer(ctx, "child-1", exp.ID, "wrong"); err != nil {
			t.Fatalf("answer %d: %v", i, err)
		}
	}
}
