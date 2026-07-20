package game

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/abhisek/mathiz/internal/store"
)

// fakeQuestSource is an in-memory QuestSource: one quest, per-child correct
// sets, remaining computed like the real service.
type fakeQuestSource struct {
	mu      sync.Mutex
	quest   QuestPlay       // all authored questions
	correct map[string]bool // "<child>/<questionUID>" → answered correctly
	items   []QuestMapItem  // returned by ActiveQuests
}

func newFakeQuestSource(skillID string, n int) *fakeQuestSource {
	f := &fakeQuestSource{
		quest: QuestPlay{
			QuestUID: "quest-1",
			Name:     "Captain's Quest",
			Emoji:    "📜",
			SkillID:  skillID,
		},
		correct: make(map[string]bool),
	}
	for i := 0; i < n; i++ {
		f.quest.Questions = append(f.quest.Questions, QuestPlayQuestion{
			UID:         fmt.Sprintf("qq-%d", i),
			Text:        fmt.Sprintf("Quest question %d: what is 2 + 2?", i),
			Answer:      "4",
			AnswerType:  "integer",
			Format:      "numeric",
			Hint:        "Use your fingers!",
			Explanation: "2 and 2 make 4.",
		})
	}
	return f
}

func (f *fakeQuestSource) key(childUID, questionUID string) string {
	return childUID + "/" + questionUID
}

func (f *fakeQuestSource) PlayableQuest(_ context.Context, childUID, questUID string) (*QuestPlay, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if questUID != f.quest.QuestUID {
		return nil, ErrQuestUnavailable
	}
	out := f.quest
	out.Questions = nil
	for _, q := range f.quest.Questions {
		if !f.correct[f.key(childUID, q.UID)] {
			out.Questions = append(out.Questions, q)
		}
	}
	if len(out.Questions) == 0 {
		return nil, ErrQuestDone
	}
	return &out, nil
}

func (f *fakeQuestSource) RecordAnswer(_ context.Context, questUID, childUID, questionUID string, correct bool) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if correct {
		f.correct[f.key(childUID, questionUID)] = true
	}
	remaining := 0
	for _, q := range f.quest.Questions {
		if !f.correct[f.key(childUID, q.UID)] {
			remaining++
		}
	}
	return remaining, nil
}

func (f *fakeQuestSource) ActiveQuests(_ context.Context, childUID string) ([]QuestMapItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.items, nil
}

func newQuestTestManager(t *testing.T, src QuestSource) *Manager {
	t.Helper()
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewManager(Config{
		Store: st,
		Toolset: func(ctx context.Context, eventRepo store.EventRepo) (*Toolset, error) {
			// The LLM generator must never be needed for a quest; give it
			// one that fails loudly if called.
			return &Toolset{Generator: &fakeGenerator{fail: true}}, nil
		},
		Quests: src,
	})
}

func TestQuestExpeditionServesAuthoredQuestionsInChunks(t *testing.T) {
	src := newFakeQuestSource("", 7) // untagged, 7 questions → 5 + 2
	m := newQuestTestManager(t, src)
	ctx := context.Background()

	exp, err := m.StartQuest(ctx, "child-1", "quest-1")
	if err != nil {
		t.Fatalf("start quest: %v", err)
	}
	if exp.QuestID != "quest-1" || exp.SkillName != "Captain's Quest" || exp.SkillID != "" {
		t.Errorf("expedition = %+v", exp)
	}
	if exp.TotalQuestions != 5 {
		t.Fatalf("total = %d, want 5 (min(5, 7))", exp.TotalQuestions)
	}

	// Questions are the authored ones, in order.
	var last *AnswerResultView
	for i := 0; i < 5; i++ {
		q, err := m.Question(ctx, "child-1", exp.ID)
		if err != nil {
			t.Fatalf("question %d: %v", i, err)
		}
		if q.Text != src.quest.Questions[i].Text {
			t.Errorf("question %d text = %q", i, q.Text)
		}
		if q.Total != 5 {
			t.Errorf("question %d total = %d", i, q.Total)
		}
		last, err = m.Answer(ctx, "child-1", exp.ID, "4")
		if err != nil {
			t.Fatalf("answer %d: %v", i, err)
		}
	}
	if !last.Done || last.Summary == nil {
		t.Fatalf("expedition should end after 5: %+v", last)
	}
	if last.Summary.QuestID != "quest-1" || last.Summary.QuestComplete {
		t.Errorf("summary = %+v (2 questions remain)", last.Summary)
	}

	// The next expedition serves the remaining 2 (already-correct skipped).
	exp2, err := m.StartQuest(ctx, "child-1", "quest-1")
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if exp2.TotalQuestions != 2 {
		t.Fatalf("second total = %d, want 2", exp2.TotalQuestions)
	}
	q, err := m.Question(ctx, "child-1", exp2.ID)
	if err != nil {
		t.Fatalf("question: %v", err)
	}
	if q.Text != src.quest.Questions[5].Text {
		t.Errorf("second expedition starts at %q", q.Text)
	}
	if _, err := m.Answer(ctx, "child-1", exp2.ID, "4"); err != nil {
		t.Fatalf("answer 6: %v", err)
	}
	last = answerCurrent(t, m, "child-1", exp2.ID, "4")
	if !last.Done || last.Summary == nil || !last.Summary.QuestComplete {
		t.Fatalf("last remaining answered correctly should complete the quest: %+v", last.Summary)
	}

	// A finished quest can't start another expedition.
	if _, err := m.StartQuest(ctx, "child-1", "quest-1"); !errors.Is(err, ErrQuestDone) {
		t.Errorf("start after completion: %v", err)
	}
	// Unknown quests are unavailable.
	if _, err := m.StartQuest(ctx, "child-1", "quest-404"); !errors.Is(err, ErrQuestUnavailable) {
		t.Errorf("unknown quest: %v", err)
	}
}

func TestQuestWrongAnswerKeepsQuestionInPlay(t *testing.T) {
	src := newFakeQuestSource("", 2)
	m := newQuestTestManager(t, src)
	ctx := context.Background()

	exp, err := m.StartQuest(ctx, "child-1", "quest-1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if exp.TotalQuestions != 2 {
		t.Fatalf("total = %d", exp.TotalQuestions)
	}
	// First wrong, second correct: hint works, quest not complete.
	res := answerCurrent(t, m, "child-1", exp.ID, "7")
	if res.Correct || !res.HintAvailable {
		t.Fatalf("wrong answer = %+v", res)
	}
	if _, err := m.Hint(ctx, "child-1", exp.ID); err != nil {
		t.Fatalf("hint: %v", err)
	}
	res = answerCurrent(t, m, "child-1", exp.ID, "4")
	if !res.Done || res.Summary == nil || res.Summary.QuestComplete {
		t.Fatalf("summary = %+v (question 0 still open)", res.Summary)
	}

	// The missed question comes back on the next expedition.
	exp2, err := m.StartQuest(ctx, "child-1", "quest-1")
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if exp2.TotalQuestions != 1 {
		t.Fatalf("restart total = %d", exp2.TotalQuestions)
	}
	q, _ := m.Question(ctx, "child-1", exp2.ID)
	if q.Text != src.quest.Questions[0].Text {
		t.Errorf("retry question = %q", q.Text)
	}
	res = answerCurrent(t, m, "child-1", exp2.ID, "4")
	if !res.Done || !res.Summary.QuestComplete {
		t.Fatalf("final summary = %+v", res.Summary)
	}
}

// TestQuestAnswersSealedUntilSolved guards the sealed-answer policy
// (specs/15-quests.md): quest questions repeat until solved, so a wrong
// answer must not reveal the correct answer or the explanation — a child
// could copy the reveal on the retry. The explanation is revealed on a
// correct answer (the question can never gate again), and hints still flow.
func TestQuestAnswersSealedUntilSolved(t *testing.T) {
	src := newFakeQuestSource("", 2)
	m := newQuestTestManager(t, src)
	ctx := context.Background()

	exp, err := m.StartQuest(ctx, "child-1", "quest-1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// Wrong answer: sealed — no correct answer, no explanation.
	res := answerCurrent(t, m, "child-1", exp.ID, "7")
	if res.Correct {
		t.Fatal("wrong answer graded correct")
	}
	if res.CorrectAnswer != "" {
		t.Errorf("wrong quest answer revealed CorrectAnswer = %q", res.CorrectAnswer)
	}
	if res.Explanation != "" {
		t.Errorf("wrong quest answer revealed Explanation = %q", res.Explanation)
	}
	// The learning scaffolds still flow: the hint is available and served.
	if !res.HintAvailable {
		t.Error("hint should still be available on a sealed wrong answer")
	}
	if _, err := m.Hint(ctx, "child-1", exp.ID); err != nil {
		t.Errorf("hint: %v", err)
	}

	// Correct answer: the explanation is the closure reveal.
	res = answerCurrent(t, m, "child-1", exp.ID, "4")
	if !res.Correct {
		t.Fatal("correct answer graded wrong")
	}
	if res.CorrectAnswer != "4" {
		t.Errorf("correct quest answer CorrectAnswer = %q, want 4", res.CorrectAnswer)
	}
	if res.Explanation != "2 and 2 make 4." {
		t.Errorf("correct quest answer Explanation = %q", res.Explanation)
	}
}

// TestMapDigWrongAnswerStillReveals is the regression guard for the map:
// adaptive dig questions never repeat, so a wrong answer keeps revealing the
// correct answer and the explanation (pure learning).
func TestMapDigWrongAnswerStillReveals(t *testing.T) {
	m := newTestManager(t, &fakeGenerator{})
	ctx := context.Background()

	exp, err := m.Start(ctx, "child-1", rootSkillID(t))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	res := answerCurrent(t, m, "child-1", exp.ID, "7")
	if res.Correct {
		t.Fatal("wrong answer graded correct")
	}
	if res.CorrectAnswer != "4" {
		t.Errorf("map dig wrong answer CorrectAnswer = %q, want 4", res.CorrectAnswer)
	}
	if res.Explanation != "2 and 2 together make 4." {
		t.Errorf("map dig wrong answer Explanation = %q", res.Explanation)
	}
}

func TestQuestStartChargesOnceAndReuses(t *testing.T) {
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	src := newFakeQuestSource("", 3)
	charges := 0
	m := NewManager(Config{
		Store: st,
		Toolset: func(ctx context.Context, eventRepo store.EventRepo) (*Toolset, error) {
			return &Toolset{Generator: &fakeGenerator{fail: true}}, nil
		},
		Quests: src,
		Charge: func(_ context.Context, _, sessionID string) error {
			if sessionID == "" {
				t.Error("charge without session ID")
			}
			charges++
			return nil
		},
	})
	ctx := context.Background()

	first, err := m.StartQuest(ctx, "child-1", "quest-1")
	if err != nil {
		t.Fatalf("start 1: %v", err)
	}
	second, err := m.StartQuest(ctx, "child-1", "quest-1")
	if err != nil {
		t.Fatalf("start 2: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("double-click created a second quest expedition")
	}
	if charges != 1 {
		t.Errorf("charges = %d, want 1", charges)
	}

	// An empty wallet refuses the quest like any expedition.
	m2 := NewManager(Config{
		Store: st,
		Toolset: func(ctx context.Context, eventRepo store.EventRepo) (*Toolset, error) {
			return &Toolset{Generator: &fakeGenerator{fail: true}}, nil
		},
		Quests: newFakeQuestSource("", 3),
		Charge: func(_ context.Context, _, _ string) error { return ErrNoCredits },
	})
	if _, err := m2.StartQuest(ctx, "child-2", "quest-1"); !errors.Is(err, ErrNoCredits) {
		t.Errorf("broke start: %v", err)
	}
}

func TestUntaggedQuestLeavesGraphUntouched(t *testing.T) {
	src := newFakeQuestSource("", 5)
	m := newQuestTestManager(t, src)
	ctx := context.Background()

	exp, err := m.StartQuest(ctx, "child-1", "quest-1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	var last *AnswerResultView
	for i := 0; i < 5; i++ {
		last = answerCurrent(t, m, "child-1", exp.ID, "4")
		if last.Mastery != nil {
			t.Errorf("untagged quest produced a mastery transition: %+v", last.Mastery)
		}
	}
	if !last.Done || !last.Summary.QuestComplete {
		t.Fatalf("summary = %+v", last.Summary)
	}
	// Streak gems still flow (5 correct in a row).
	if last.Gem == nil {
		t.Error("expected a streak gem on the 5th correct answer")
	}

	// The skill graph is untouched: the map looks factory-fresh apart from
	// the gems earned.
	mv, err := m.Map(ctx, "child-1")
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	for _, island := range mv.Islands {
		for _, spot := range island.Spots {
			if spot.State != "ready" && spot.State != "locked" {
				t.Errorf("spot %s = %s after untagged quest", spot.ID, spot.State)
			}
			if spot.Progress != 0 {
				t.Errorf("spot %s progress = %f", spot.ID, spot.Progress)
			}
		}
	}
	if mv.Gems.Total == 0 {
		t.Error("gems should persist for quest play")
	}
}

func TestTaggedQuestAdvancesMastery(t *testing.T) {
	root := rootSkillID(t)
	src := newFakeQuestSource(root, 5)
	m := newQuestTestManager(t, src)
	ctx := context.Background()

	exp, err := m.StartQuest(ctx, "child-1", "quest-1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if exp.SkillID != root {
		t.Errorf("tagged expedition skill = %q", exp.SkillID)
	}
	for i := 0; i < 5; i++ {
		answerCurrent(t, m, "child-1", exp.ID, "4")
	}

	// Quest practice pushed the main map forward.
	mv, err := m.Map(ctx, "child-1")
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	spot := findSpot(t, mv, root)
	if spot.State != "digging" || spot.Progress <= 0 {
		t.Errorf("root after tagged quest = %+v", spot)
	}
}

func TestMapListsActiveQuests(t *testing.T) {
	src := newFakeQuestSource("", 3)
	src.items = []QuestMapItem{{
		ID: "quest-1", Name: "Captain's Quest", Emoji: "📜", Total: 3, Correct: 1,
	}}
	m := newQuestTestManager(t, src)

	mv, err := m.Map(context.Background(), "child-1")
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if len(mv.Quests) != 1 || mv.Quests[0].ID != "quest-1" || mv.Quests[0].Correct != 1 {
		t.Errorf("map quests = %+v", mv.Quests)
	}

	// Without a quest source the map simply has no quests.
	m2 := newTestManager(t, &fakeGenerator{})
	mv2, err := m2.Map(context.Background(), "child-1")
	if err != nil {
		t.Fatalf("map without quests: %v", err)
	}
	if mv2.Quests != nil {
		t.Errorf("quests without source = %+v", mv2.Quests)
	}
}

func TestQuestExpeditionOwnership(t *testing.T) {
	src := newFakeQuestSource("", 3)
	m := newQuestTestManager(t, src)
	ctx := context.Background()

	exp, err := m.StartQuest(ctx, "child-1", "quest-1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := m.Question(ctx, "child-2", exp.ID); !errors.Is(err, ErrNoExpedition) {
		t.Errorf("cross-child question: %v", err)
	}
	if _, err := m.Answer(ctx, "child-2", exp.ID, "4"); !errors.Is(err, ErrNoExpedition) {
		t.Errorf("cross-child answer: %v", err)
	}

	// StartQuest disabled entirely without a source.
	m2 := newTestManager(t, &fakeGenerator{})
	if _, err := m2.StartQuest(ctx, "child-1", "quest-1"); !errors.Is(err, ErrQuestUnavailable) {
		t.Errorf("start without source: %v", err)
	}
}

// TestQuestThenSkillStartDoesNotReuse guards the reuse check: an untouched
// quest expedition must not satisfy a skill Start (and vice versa).
func TestQuestThenSkillStartDoesNotReuse(t *testing.T) {
	root := rootSkillID(t)
	src := newFakeQuestSource(root, 3)
	m := newQuestTestManager(t, src)
	ctx := context.Background()

	qexp, err := m.StartQuest(ctx, "child-1", "quest-1")
	if err != nil {
		t.Fatalf("start quest: %v", err)
	}
	sexp, err := m.Start(ctx, "child-1", root)
	if err != nil {
		t.Fatalf("start skill: %v", err)
	}
	if sexp.ID == qexp.ID {
		t.Fatal("skill Start reused a quest expedition")
	}
	if sexp.QuestID != "" {
		t.Errorf("skill expedition has quest ID %q", sexp.QuestID)
	}
}
