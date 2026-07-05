package game

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
)

// rootSkillID is the single root of the skill graph (no prerequisites).
func rootSkillID(t *testing.T) string {
	t.Helper()
	roots := skillgraph.RootSkills()
	if len(roots) != 1 {
		t.Fatalf("expected exactly 1 root skill, got %d", len(roots))
	}
	return roots[0].ID
}

// fakeGenerator returns deterministic questions; the answer is always "4".
type fakeGenerator struct {
	calls int
	fail  bool
}

func (f *fakeGenerator) Generate(_ context.Context, input problemgen.GenerateInput) (*problemgen.Question, error) {
	if f.fail {
		return nil, errors.New("llm unavailable")
	}
	f.calls++
	return &problemgen.Question{
		Text:        fmt.Sprintf("What is 2 + 2? (v%d)", f.calls),
		Format:      problemgen.FormatNumeric,
		Answer:      "4",
		AnswerType:  problemgen.AnswerTypeInteger,
		Hint:        "Count on your fingers!",
		Explanation: "2 and 2 together make 4.",
		SkillID:     input.Skill.ID,
		Tier:        input.Tier,
	}, nil
}

func newTestManager(t *testing.T, gen problemgen.Generator) *Manager {
	t.Helper()
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewManager(Config{
		Store: st,
		Toolset: func(ctx context.Context, eventRepo store.EventRepo) (*Toolset, error) {
			return &Toolset{Generator: gen}, nil
		},
	})
}

// answerCurrent asks for the next question and answers it.
func answerCurrent(t *testing.T, m *Manager, child, expID, answer string) *AnswerResultView {
	t.Helper()
	ctx := context.Background()
	if _, err := m.Question(ctx, child, expID); err != nil {
		t.Fatalf("question: %v", err)
	}
	res, err := m.Answer(ctx, child, expID, answer)
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	return res
}

func TestExpeditionHappyPath(t *testing.T) {
	m := newTestManager(t, &fakeGenerator{})
	ctx := context.Background()
	root := rootSkillID(t)

	exp, err := m.Start(ctx, "child-1", root)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if exp.TotalQuestions != QuestionsPerExpedition || exp.Tier != "learn" || exp.Category != "frontier" {
		t.Errorf("expedition = %+v", exp)
	}

	var last *AnswerResultView
	for i := 1; i <= QuestionsPerExpedition; i++ {
		last = answerCurrent(t, m, "child-1", exp.ID, "4")
		if !last.Correct {
			t.Fatalf("q%d graded wrong", i)
		}
		if last.QuestionsAnswered != i {
			t.Errorf("q%d: answered = %d", i, last.QuestionsAnswered)
		}
	}
	if !last.Done || last.Summary == nil {
		t.Fatalf("expedition should be done: %+v", last)
	}
	if last.Summary.Questions != 5 || last.Summary.Correct != 5 {
		t.Errorf("summary = %+v", last.Summary)
	}
	// 5 consecutive correct answers = streak gem (threshold 5) surfaced on
	// the final answer, and a session gem in the summary.
	if last.Gem == nil {
		t.Error("expected a streak gem on the 5th correct answer")
	}
	hasSession := false
	for _, g := range last.Summary.Gems {
		if g.Type == "session" {
			hasSession = true
		}
	}
	if !hasSession {
		t.Errorf("expected session gem in summary gems: %+v", last.Summary.Gems)
	}

	// The expedition is gone afterwards.
	if _, err := m.Question(ctx, "child-1", exp.ID); !errors.Is(err, ErrNoExpedition) {
		t.Errorf("finished expedition still reachable: %v", err)
	}

	// Progress persisted: the map shows the root as digging with progress.
	mv, err := m.Map(ctx, "child-1")
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	spot := findSpot(t, mv, root)
	if spot.State != "digging" || spot.Progress <= 0 {
		t.Errorf("root spot after expedition = %+v", spot)
	}
}

func TestMasteryOpensChestAndLiftsFog(t *testing.T) {
	m := newTestManager(t, &fakeGenerator{})
	ctx := context.Background()
	root := rootSkillID(t)

	// Answer correctly until mastered: learn tier (8 problems) then prove
	// tier (6 problems). Mastery ends an expedition early.
	var mastered *AnswerResultView
	for round := 0; round < 6 && mastered == nil; round++ {
		exp, err := m.Start(ctx, "child-1", root)
		if err != nil {
			t.Fatalf("start round %d: %v", round, err)
		}
		for q := 0; q < QuestionsPerExpedition; q++ {
			res := answerCurrent(t, m, "child-1", exp.ID, "4")
			if res.Mastery != nil && res.Mastery.To == "mastered" {
				mastered = res
				break
			}
			if res.Done {
				break
			}
		}
	}
	if mastered == nil {
		t.Fatal("never mastered the root skill")
	}
	if !mastered.Done || mastered.Summary == nil || !mastered.Summary.Mastered {
		t.Errorf("mastering answer should end the expedition triumphantly: %+v", mastered)
	}
	if len(mastered.UnlockedSkillIDs) == 0 {
		t.Error("mastering the root should lift fog on dependents")
	}
	if mastered.Gem == nil || mastered.Gem.Type != "mastery" {
		t.Errorf("expected mastery gem, got %+v", mastered.Gem)
	}

	// Map: chest open, dependents ready.
	mv, err := m.Map(ctx, "child-1")
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if spot := findSpot(t, mv, root); spot.State != "treasure" {
		t.Errorf("root spot = %+v, want treasure", spot)
	}
	for _, id := range mastered.UnlockedSkillIDs {
		if spot := findSpot(t, mv, id); spot.State != "ready" {
			t.Errorf("unlocked spot %s = %s, want ready", id, spot.State)
		}
	}
}

func TestLockedSpotRefused(t *testing.T) {
	m := newTestManager(t, &fakeGenerator{})
	ctx := context.Background()

	// Any non-root skill is locked for a fresh child.
	var locked string
	for _, s := range skillgraph.AllSkills() {
		if len(s.Prerequisites) > 0 {
			locked = s.ID
			break
		}
	}
	if _, err := m.Start(ctx, "child-1", locked); !errors.Is(err, ErrLocked) {
		t.Errorf("locked start: got %v", err)
	}
	if _, err := m.Start(ctx, "child-1", "no-such-skill"); !errors.Is(err, ErrLocked) {
		t.Errorf("unknown skill: got %v", err)
	}
}

func TestWrongAnswerFlow(t *testing.T) {
	m := newTestManager(t, &fakeGenerator{})
	ctx := context.Background()
	root := rootSkillID(t)

	exp, err := m.Start(ctx, "child-1", root)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	res := answerCurrent(t, m, "child-1", exp.ID, "7")
	if res.Correct {
		t.Fatal("wrong answer graded correct")
	}
	if res.CorrectAnswer != "4" || res.Explanation == "" {
		t.Errorf("feedback = %+v", res)
	}
	if !res.HintAvailable {
		t.Fatal("hint should be offered after a wrong answer")
	}
	if res.Streak != 0 {
		t.Errorf("streak = %d after wrong answer", res.Streak)
	}

	hint, err := m.Hint(ctx, "child-1", exp.ID)
	if err != nil {
		t.Fatalf("hint: %v", err)
	}
	if !strings.Contains(hint.Hint, "fingers") {
		t.Errorf("hint = %q", hint.Hint)
	}
	// Hint is one-shot.
	if _, err := m.Hint(ctx, "child-1", exp.ID); !errors.Is(err, ErrNoHint) {
		t.Errorf("second hint: got %v", err)
	}

	// Double answer without a fresh question is rejected.
	if _, err := m.Answer(ctx, "child-1", exp.ID, "4"); !errors.Is(err, ErrNoQuestion) {
		t.Errorf("double answer: got %v", err)
	}
}

func TestMicroLessonFlow(t *testing.T) {
	// A lessons service backed by the mock LLM provider delivers one lesson.
	lessonJSON := `{
		"title": "Counting On",
		"explanation": "When adding small numbers, start from the bigger one and count up.",
		"worked_example": "For 2 + 2: start at 2, count 3, 4. The answer is 4.",
		"practice_question": {"text": "Try it: what is 2 + 3?", "answer": "5", "answer_type": "integer", "explanation": "Start at 3: count 4, 5."}
	}`
	lessonSvc := lessons.NewService(
		llm.NewMockProvider(llm.MockResponse{Content: []byte(lessonJSON)}),
		lessons.DefaultConfig(),
	)

	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	m := NewManager(Config{
		Store: st,
		Toolset: func(ctx context.Context, eventRepo store.EventRepo) (*Toolset, error) {
			return &Toolset{Generator: &fakeGenerator{}, Lessons: lessonSvc}, nil
		},
	})
	ctx := context.Background()
	root := rootSkillID(t)

	exp, err := m.Start(ctx, "child-1", root)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// One wrong answer: no lesson yet.
	res := answerCurrent(t, m, "child-1", exp.ID, "7")
	if res.LessonPending {
		t.Error("lesson pending after a single wrong answer")
	}
	if _, err := m.Lesson(ctx, "child-1", exp.ID); !errors.Is(err, ErrNoLesson) {
		t.Errorf("lesson before pending: got %v", err)
	}

	// Second wrong answer triggers the guide.
	res = answerCurrent(t, m, "child-1", exp.ID, "7")
	if !res.LessonPending {
		t.Fatal("lesson should be pending after two wrong answers")
	}

	// Poll until the async generation lands.
	var lesson *LessonView
	for i := 0; i < 50; i++ {
		lesson, err = m.Lesson(ctx, "child-1", exp.ID)
		if err != nil {
			t.Fatalf("lesson: %v", err)
		}
		if lesson.Ready {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !lesson.Ready || lesson.Title != "Counting On" || lesson.Practice == nil {
		t.Fatalf("lesson = %+v", lesson)
	}

	// Practice: wrong answer graded, correct answer revealed.
	graded, err := m.AnswerLesson(ctx, "child-1", exp.ID, "9", false)
	if err != nil {
		t.Fatalf("answer lesson: %v", err)
	}
	if graded.Correct || graded.CorrectAnswer != "5" {
		t.Errorf("graded = %+v", graded)
	}
	// Lesson is consumed.
	if _, err := m.AnswerLesson(ctx, "child-1", exp.ID, "5", false); !errors.Is(err, ErrNoLesson) {
		t.Errorf("second lesson answer: got %v", err)
	}

	// The expedition continues afterwards.
	res = answerCurrent(t, m, "child-1", exp.ID, "4")
	if !res.Correct {
		t.Error("expedition broken after lesson")
	}

	// The tip lives on in the guide's notebook, with full content and the
	// island it belongs to.
	nb, err := m.Notebook(ctx, "child-1")
	if err != nil {
		t.Fatalf("notebook: %v", err)
	}
	if len(nb.Tips) != 1 {
		t.Fatalf("notebook tips = %d, want 1", len(nb.Tips))
	}
	tip := nb.Tips[0]
	if tip.Title != "Counting On" || tip.SkillID != root {
		t.Errorf("tip = %+v", tip)
	}
	if tip.Explanation == "" || tip.WorkedExample == "" || tip.PracticeAnswer != "5" {
		t.Errorf("tip content missing: %+v", tip)
	}
	if tip.IslandName == "" || tip.SkillName == root {
		t.Errorf("tip not enriched with skill metadata: %+v", tip)
	}

	// Another child's notebook is empty (owner scoping).
	other, err := m.Notebook(ctx, "child-2")
	if err != nil {
		t.Fatalf("notebook child-2: %v", err)
	}
	if len(other.Tips) != 0 {
		t.Errorf("cross-child notebook tips = %d, want 0", len(other.Tips))
	}
}

func TestExpeditionOwnership(t *testing.T) {
	m := newTestManager(t, &fakeGenerator{})
	ctx := context.Background()
	root := rootSkillID(t)

	exp, err := m.Start(ctx, "child-1", root)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := m.Question(ctx, "child-2", exp.ID); !errors.Is(err, ErrNoExpedition) {
		t.Errorf("cross-child question: got %v", err)
	}
	if _, err := m.Answer(ctx, "child-2", exp.ID, "4"); !errors.Is(err, ErrNoExpedition) {
		t.Errorf("cross-child answer: got %v", err)
	}
	if _, err := m.End(ctx, "child-2", exp.ID); !errors.Is(err, ErrNoExpedition) {
		t.Errorf("cross-child end: got %v", err)
	}
}

func TestStartReplacesActiveExpedition(t *testing.T) {
	m := newTestManager(t, &fakeGenerator{})
	ctx := context.Background()
	root := rootSkillID(t)

	first, err := m.Start(ctx, "child-1", root)
	if err != nil {
		t.Fatalf("start 1: %v", err)
	}
	answerCurrent(t, m, "child-1", first.ID, "4")

	second, err := m.Start(ctx, "child-1", root)
	if err != nil {
		t.Fatalf("start 2: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("expected a fresh expedition")
	}
	// The first expedition was retired (its progress saved).
	if _, err := m.Question(ctx, "child-1", first.ID); !errors.Is(err, ErrNoExpedition) {
		t.Errorf("first expedition still live: %v", err)
	}
	// Its answer survived into mastery progress.
	mv, err := m.Map(ctx, "child-1")
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if spot := findSpot(t, mv, root); spot.State != "digging" {
		t.Errorf("root after replaced expedition = %+v", spot)
	}
}

func TestGenerationCircuitBreaker(t *testing.T) {
	gen := &fakeGenerator{fail: true}
	m := newTestManager(t, gen)
	ctx := context.Background()
	root := rootSkillID(t)

	exp, err := m.Start(ctx, "child-1", root)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	for i := 0; i < maxGenFailures; i++ {
		if _, err := m.Question(ctx, "child-1", exp.ID); !errors.Is(err, ErrGeneration) {
			t.Fatalf("attempt %d: got %v", i, err)
		}
	}
	// After the breaker trips the expedition is gone.
	if _, err := m.Question(ctx, "child-1", exp.ID); !errors.Is(err, ErrNoExpedition) {
		t.Errorf("after breaker: got %v", err)
	}
}

func TestFreshMapState(t *testing.T) {
	m := newTestManager(t, &fakeGenerator{})
	mv, err := m.Map(context.Background(), "child-new")
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if len(mv.Islands) != len(skillgraph.AllStrands()) {
		t.Fatalf("islands = %d", len(mv.Islands))
	}
	total, ready, locked := 0, 0, 0
	for _, island := range mv.Islands {
		for _, spot := range island.Spots {
			total++
			switch spot.State {
			case "ready":
				ready++
			case "locked":
				locked++
			}
		}
	}
	if total != len(skillgraph.AllSkills()) {
		t.Errorf("spots = %d, want %d", total, len(skillgraph.AllSkills()))
	}
	if ready != 1 {
		t.Errorf("ready spots = %d, want 1 (single root)", ready)
	}
	if ready+locked != total {
		t.Errorf("fresh map has non-locked non-ready spots")
	}
	if mv.Gems.Total != 0 {
		t.Errorf("fresh gems = %d", mv.Gems.Total)
	}
}

func findSpot(t *testing.T, mv *MapView, id string) SpotView {
	t.Helper()
	for _, island := range mv.Islands {
		for _, spot := range island.Spots {
			if spot.ID == id {
				return spot
			}
		}
	}
	t.Fatalf("spot %s not found", id)
	return SpotView{}
}
