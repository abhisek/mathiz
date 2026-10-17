package problemgen

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/skillgraph"
)

// gateGen blocks every Generate until the test releases it (one token per
// call) or the call's context ends, and records what it saw.
type gateGen struct {
	mu        sync.Mutex
	calls     int
	cancelled int
	inputs    []GenerateInput
	failCall  map[int]bool // 1-based call numbers that fail

	started chan int
	release chan struct{}
}

func newGateGen() *gateGen {
	return &gateGen{started: make(chan int, 16), release: make(chan struct{}, 16), failCall: map[int]bool{}}
}

func (g *gateGen) Generate(ctx context.Context, in GenerateInput) (*Question, error) {
	g.mu.Lock()
	g.calls++
	n := g.calls
	g.inputs = append(g.inputs, in)
	fail := g.failCall[n]
	g.mu.Unlock()
	g.started <- n
	select {
	case <-g.release:
	case <-ctx.Done():
		g.mu.Lock()
		g.cancelled++
		g.mu.Unlock()
		return nil, ctx.Err()
	}
	if fail {
		return nil, fmt.Errorf("call %d failed", n)
	}
	return &Question{Text: fmt.Sprintf("q%d", n)}, nil
}

func (g *gateGen) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func (g *gateGen) cancelCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.cancelled
}

// waitStarted waits for the next generator call to begin.
func (g *gateGen) waitStarted(t *testing.T) int {
	t.Helper()
	select {
	case n := <-g.started:
		return n
	case <-time.After(2 * time.Second):
		t.Fatal("generator call never started")
		return 0
	}
}

func input(prior ...string) GenerateInput {
	return GenerateInput{Skill: skillgraph.Skill{ID: "add-3digit"}, Tier: skillgraph.TierLearn, PriorQuestions: prior}
}

func newTestPrefetcher(t *testing.T, g Generator) *Prefetcher {
	t.Helper()
	p := NewPrefetcher(g, PrefetchConfig{Budget: 5 * time.Second})
	t.Cleanup(p.Close)
	return p
}

// eventually polls cond, for state that a background goroutine settles.
func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal("condition never held")
}

func TestPrefetchHit(t *testing.T) {
	g := newGateGen()
	p := newTestPrefetcher(t, g)
	p.Warm(input("a"))
	p.Warm(input("a")) // identical: no second call
	g.waitStarted(t)
	g.release <- struct{}{}
	eventually(t, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		select {
		case <-p.entry.done:
			return true
		default:
			return false
		}
	})

	res, err := p.Get(context.Background(), input("a"))
	if err != nil || res.Source != SourcePrefetchHit || res.Question.Text != "q1" {
		t.Fatalf("Get = %+v, %v; want q1 from %s", res, err, SourcePrefetchHit)
	}
	if g.callCount() != 1 {
		t.Errorf("calls = %d, want 1", g.callCount())
	}
}

func TestPrefetchJoin(t *testing.T) {
	g := newGateGen()
	p := newTestPrefetcher(t, g)
	p.Warm(input("a"))
	g.waitStarted(t)

	got := make(chan Result, 1)
	go func() {
		res, _ := p.Get(context.Background(), input("a"))
		got <- res
	}()
	time.Sleep(10 * time.Millisecond) // let Get claim the in-flight warm
	g.release <- struct{}{}
	res := <-got
	if res.Source != SourcePrefetchJoin || res.Question == nil || res.Question.Text != "q1" {
		t.Fatalf("Get = %+v, want q1 from %s", res, SourcePrefetchJoin)
	}
	if g.callCount() != 1 {
		t.Errorf("calls = %d, want 1", g.callCount())
	}
}

func TestPrefetchMissCancelsStaleWarm(t *testing.T) {
	g := newGateGen()
	p := newTestPrefetcher(t, g)
	p.Warm(input("a"))
	g.waitStarted(t)

	got := make(chan Result, 1)
	go func() {
		res, _ := p.Get(context.Background(), input("a", "b")) // the input changed
		got <- res
	}()
	g.waitStarted(t)
	g.release <- struct{}{}
	res := <-got
	if res.Source != SourceFresh || res.Question.Text != "q2" {
		t.Fatalf("Get = %+v, want q2 from %s", res, SourceFresh)
	}
	eventually(t, func() bool { return g.cancelCount() == 1 })
}

func TestPrefetchFailedWarmFallsBackToFresh(t *testing.T) {
	for _, joined := range []bool{false, true} {
		t.Run(fmt.Sprintf("joined=%v", joined), func(t *testing.T) {
			g := newGateGen()
			g.failCall[1] = true
			p := newTestPrefetcher(t, g)
			p.Warm(input("a"))
			g.waitStarted(t)
			if !joined {
				g.release <- struct{}{}
				eventually(t, func() bool {
					p.mu.Lock()
					defer p.mu.Unlock()
					select {
					case <-p.entry.done:
						return true
					default:
						return false
					}
				})
			}

			got := make(chan Result, 1)
			errc := make(chan error, 1)
			go func() {
				res, err := p.Get(context.Background(), input("a"))
				got <- res
				errc <- err
			}()
			if joined {
				time.Sleep(10 * time.Millisecond)
				g.release <- struct{}{} // the joined warm fails
			}
			g.waitStarted(t) // the fresh retry
			g.release <- struct{}{}
			res, err := <-got, <-errc
			if err != nil || res.Source != SourceFresh || res.Question.Text != "q2" {
				t.Fatalf("Get = %+v, %v; want q2 from %s", res, err, SourceFresh)
			}
		})
	}
}

func TestPrefetchFreshFailureIsReturned(t *testing.T) {
	g := newGateGen()
	g.failCall[1] = true
	p := newTestPrefetcher(t, g)
	go func() {
		<-g.started
		g.release <- struct{}{}
	}()
	if _, err := p.Get(context.Background(), input("a")); err == nil {
		t.Fatal("expected the fresh generation's error")
	}
	// The failed entry is forgotten: the next Get tries again.
	go func() {
		<-g.started
		g.release <- struct{}{}
	}()
	res, err := p.Get(context.Background(), input("a"))
	if err != nil || res.Question.Text != "q2" {
		t.Fatalf("retry = %+v, %v", res, err)
	}
}

func TestPrefetchConcurrentGetsShareOneCall(t *testing.T) {
	g := newGateGen()
	p := newTestPrefetcher(t, g)
	var wg sync.WaitGroup
	results := make([]Result, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = p.Get(context.Background(), input("a"))
		}()
	}
	g.waitStarted(t)
	time.Sleep(10 * time.Millisecond)
	g.release <- struct{}{}
	wg.Wait()
	if g.callCount() != 1 || results[0].Question != results[1].Question {
		t.Fatalf("calls = %d, results %+v", g.callCount(), results)
	}

	// A late duplicate (after delivery) gets the same question, not a call.
	late, err := p.Get(context.Background(), input("a"))
	if err != nil || late.Question != results[0].Question || g.callCount() != 1 {
		t.Fatalf("late duplicate = %+v, %v, calls %d", late, err, g.callCount())
	}
}

func TestPrefetchCallerGivesUpGenerationSurvives(t *testing.T) {
	g := newGateGen()
	p := newTestPrefetcher(t, g)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := p.Get(ctx, input("a")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Get = %v, want the caller's deadline", err)
	}
	g.waitStarted(t)
	g.release <- struct{}{}
	res, err := p.Get(context.Background(), input("a"))
	if err != nil || res.Question.Text != "q1" || g.callCount() != 1 {
		t.Fatalf("second Get = %+v, %v, calls %d; want the surviving q1", res, err, g.callCount())
	}
	if g.cancelCount() != 0 {
		t.Error("the first caller's exit cancelled the shared generation")
	}
}

func TestPrefetchCloseCancelsInFlight(t *testing.T) {
	g := newGateGen()
	p := NewPrefetcher(g, PrefetchConfig{Budget: 5 * time.Second})
	p.Warm(input("a"))
	g.waitStarted(t)

	done := make(chan struct{})
	go func() {
		p.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close blocked on the in-flight generation")
	}
	eventually(t, func() bool { return g.cancelCount() == 1 })
	p.Close() // idempotent

	if _, err := p.Get(context.Background(), input("a")); !errors.Is(err, ErrPrefetcherClosed) {
		t.Errorf("Get after Close = %v", err)
	}
	p.Warm(input("b"))
	if g.callCount() != 1 {
		t.Errorf("Warm after Close started a call: %d calls", g.callCount())
	}
}

func TestPrefetchBudget(t *testing.T) {
	g := newGateGen()
	p := NewPrefetcher(g, PrefetchConfig{Budget: 30 * time.Millisecond})
	t.Cleanup(p.Close)
	if _, err := p.Get(context.Background(), input("a")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Get = %v, want the budget deadline", err)
	}
}

func TestPrefetchDisableWarm(t *testing.T) {
	g := newGateGen()
	p := NewPrefetcher(g, PrefetchConfig{Budget: time.Second, DisableWarm: true})
	t.Cleanup(p.Close)
	p.Warm(input("a"))
	time.Sleep(10 * time.Millisecond)
	if g.callCount() != 0 {
		t.Fatalf("Warm ran with DisableWarm: %d calls", g.callCount())
	}
}

func TestPrefetchClonesInput(t *testing.T) {
	g := newGateGen()
	p := newTestPrefetcher(t, g)
	prior := []string{"a", "b"}
	p.Warm(input(prior...))
	g.waitStarted(t)
	prior[0] = "changed"
	g.mu.Lock()
	seen := g.inputs[0].PriorQuestions[0]
	g.mu.Unlock()
	if seen != "a" {
		t.Errorf("generator saw %q: the background call shares the caller's slice", seen)
	}
	g.release <- struct{}{}
}

func TestFingerprint(t *testing.T) {
	base := GenerateInput{
		Skill:          skillgraph.Skill{ID: "s"},
		Tier:           skillgraph.TierLearn,
		PriorQuestions: []string{"ab", "c"},
		RecentErrors:   []string{"e"},
		LearnerProfile: "likes pirates",
	}
	if fingerprint(base) != fingerprint(base.clone()) {
		t.Fatal("identical inputs differ")
	}
	variants := map[string]func(*GenerateInput){
		"skill":          func(in *GenerateInput) { in.Skill.ID = "t" },
		"tier":           func(in *GenerateInput) { in.Tier = skillgraph.TierProve },
		"prior":          func(in *GenerateInput) { in.PriorQuestions = append(in.PriorQuestions, "d") },
		"prior boundary": func(in *GenerateInput) { in.PriorQuestions = []string{"a", "bc"} },
		"errors":         func(in *GenerateInput) { in.RecentErrors = nil },
		"profile":        func(in *GenerateInput) { in.LearnerProfile = "likes dragons" },
		"list boundary":  func(in *GenerateInput) { in.PriorQuestions, in.RecentErrors = []string{"ab"}, []string{"c", "e"} },
	}
	for name, mutate := range variants {
		in := base.clone()
		mutate(&in)
		if fingerprint(in) == fingerprint(base) {
			t.Errorf("%s change kept the fingerprint", name)
		}
	}
}
