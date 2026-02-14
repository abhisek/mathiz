package session

import (
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/skillgraph"
)

func testPlan() *Plan {
	// Use all skills from the graph for test data.
	skills := skillgraph.AllSkills()
	if len(skills) < 2 {
		panic("need at least 2 skills for tests")
	}
	return &Plan{
		Slots: []PlanSlot{
			{Skill: skills[0], Tier: skillgraph.TierLearn, Category: CategoryFrontier},
			{Skill: skills[1], Tier: skillgraph.TierLearn, Category: CategoryFrontier},
		},
		Duration: DefaultSessionDuration,
	}
}

func testState() *SessionState {
	plan := testPlan()
	return NewSessionState(plan, "test-session-id", nil, nil)
}

func TestSlotCycling_AfterThreeQuestions(t *testing.T) {
	state := testState()
	state.QuestionsInSlot = QuestionsPerSlot

	if !ShouldAdvanceSlot(state) {
		t.Error("expected ShouldAdvanceSlot to return true after 3 questions")
	}

	if !AdvanceSlot(state) {
		t.Error("expected AdvanceSlot to succeed")
	}

	if state.CurrentSlotIndex != 1 {
		t.Errorf("CurrentSlotIndex = %d, want 1", state.CurrentSlotIndex)
	}
	if state.QuestionsInSlot != 0 {
		t.Errorf("QuestionsInSlot = %d, want 0", state.QuestionsInSlot)
	}
}

func TestSlotCycling_Wraparound(t *testing.T) {
	state := testState()
	state.CurrentSlotIndex = len(state.Plan.Slots) - 1

	if !AdvanceSlot(state) {
		t.Error("expected AdvanceSlot to succeed on wraparound")
	}

	if state.CurrentSlotIndex != 0 {
		t.Errorf("CurrentSlotIndex = %d, want 0 (wraparound)", state.CurrentSlotIndex)
	}
}

func TestSlotCycling_SkipCompletedTier(t *testing.T) {
	state := testState()
	state.CompletedSlots[0] = true
	state.CurrentSlotIndex = 0

	if !AdvanceSlot(state) {
		t.Error("expected AdvanceSlot to succeed (should skip completed slot)")
	}

	if state.CurrentSlotIndex != 1 {
		t.Errorf("CurrentSlotIndex = %d, want 1 (skip completed)", state.CurrentSlotIndex)
	}
}

func TestSlotCycling_AllCompleted(t *testing.T) {
	state := testState()
	for i := range state.Plan.Slots {
		state.CompletedSlots[i] = true
	}

	if AdvanceSlot(state) {
		t.Error("expected AdvanceSlot to return false when all slots completed")
	}
}

func TestHandleAnswer_Correct(t *testing.T) {
	state := testState()
	skill := state.Plan.Slots[0].Skill
	state.CurrentQuestion = &problemgen.Question{
		Text:       "What is 1 + 1?",
		Format:     problemgen.FormatNumeric,
		Answer:     "2",
		AnswerType: problemgen.AnswerTypeInteger,
		SkillID:    skill.ID,
		Tier:       skillgraph.TierLearn,
	}
	state.QuestionStartTime = time.Now()

	HandleAnswer(state, "2")

	if state.TotalCorrect != 1 {
		t.Errorf("TotalCorrect = %d, want 1", state.TotalCorrect)
	}
	if state.TotalQuestions != 1 {
		t.Errorf("TotalQuestions = %d, want 1", state.TotalQuestions)
	}
	if !state.LastAnswerCorrect {
		t.Error("expected LastAnswerCorrect to be true")
	}

	sr := state.PerSkillResults[skill.ID]
	if sr == nil {
		t.Fatal("expected PerSkillResults to have an entry")
	}
	if sr.Correct != 1 {
		t.Errorf("SkillResult.Correct = %d, want 1", sr.Correct)
	}
}

func TestHandleAnswer_Incorrect(t *testing.T) {
	state := testState()
	skill := state.Plan.Slots[0].Skill
	state.CurrentQuestion = &problemgen.Question{
		Text:       "What is 1 + 1?",
		Format:     problemgen.FormatNumeric,
		Answer:     "2",
		AnswerType: problemgen.AnswerTypeInteger,
		SkillID:    skill.ID,
		Tier:       skillgraph.TierLearn,
	}

	HandleAnswer(state, "3")

	if state.LastAnswerCorrect {
		t.Error("expected LastAnswerCorrect to be false")
	}
	if state.TotalCorrect != 0 {
		t.Errorf("TotalCorrect = %d, want 0", state.TotalCorrect)
	}

	errors := state.RecentErrors[skill.ID]
	if len(errors) != 1 {
		t.Errorf("RecentErrors length = %d, want 1", len(errors))
	}
}

func TestHandleAnswer_TierAdvancement(t *testing.T) {
	state := testState()
	skill := state.Plan.Slots[0].Skill

	// Setup tier progress close to completion.
	state.TierProgress[skill.ID] = &TierProgress{
		SkillID:       skill.ID,
		CurrentTier:   skillgraph.TierLearn,
		TotalAttempts: 7,
		CorrectCount:  6,
		Accuracy:      6.0 / 7.0,
	}

	state.CurrentQuestion = &problemgen.Question{
		Text:       "What is 1 + 1?",
		Format:     problemgen.FormatNumeric,
		Answer:     "2",
		AnswerType: problemgen.AnswerTypeInteger,
		SkillID:    skill.ID,
		Tier:       skillgraph.TierLearn,
	}

	adv := HandleAnswer(state, "2")

	if adv == nil {
		t.Fatal("expected tier advancement")
	}
	if adv.FromTier != skillgraph.TierLearn {
		t.Errorf("FromTier = %d, want TierLearn", adv.FromTier)
	}
	if adv.ToTier != skillgraph.TierProve {
		t.Errorf("ToTier = %d, want TierProve", adv.ToTier)
	}
	if adv.Mastered {
		t.Error("expected Mastered to be false for Learn→Prove")
	}

	// Check that tier progress was reset for the new tier.
	tp := state.TierProgress[skill.ID]
	if tp.CurrentTier != skillgraph.TierProve {
		t.Errorf("CurrentTier = %d, want TierProve", tp.CurrentTier)
	}
	if tp.TotalAttempts != 0 {
		t.Errorf("TotalAttempts = %d, want 0 (reset)", tp.TotalAttempts)
	}
}

func TestHandleAnswer_Mastery(t *testing.T) {
	state := testState()
	skill := state.Plan.Slots[0].Skill

	// Setup tier progress at Prove tier close to completion.
	state.TierProgress[skill.ID] = &TierProgress{
		SkillID:       skill.ID,
		CurrentTier:   skillgraph.TierProve,
		TotalAttempts: 5,
		CorrectCount:  5,
		Accuracy:      1.0,
	}

	state.CurrentQuestion = &problemgen.Question{
		Text:       "What is 1 + 1?",
		Format:     problemgen.FormatNumeric,
		Answer:     "2",
		AnswerType: problemgen.AnswerTypeInteger,
		SkillID:    skill.ID,
		Tier:       skillgraph.TierProve,
	}

	adv := HandleAnswer(state, "2")

	if adv == nil {
		t.Fatal("expected tier advancement")
	}
	if !adv.Mastered {
		t.Error("expected Mastered to be true")
	}
	if !state.Mastered[skill.ID] {
		t.Error("expected skill to be in mastered set")
	}
}

func TestErrorContext_Construction(t *testing.T) {
	q := &problemgen.Question{
		Text:   "What is 567 + 285?",
		Answer: "852",
	}

	result := BuildErrorContext(q, "842")
	expected := "Answered 842 for 'What is 567 + 285?', correct answer was 852"

	if result != expected {
		t.Errorf("BuildErrorContext = %q, want %q", result, expected)
	}
}

func TestErrorContext_Limit(t *testing.T) {
	state := testState()
	skill := state.Plan.Slots[0].Skill

	// Add more than MaxRecentErrors errors.
	for i := 0; i < MaxRecentErrors+3; i++ {
		state.CurrentQuestion = &problemgen.Question{
			Text:       "What is 1 + 1?",
			Format:     problemgen.FormatNumeric,
			Answer:     "2",
			AnswerType: problemgen.AnswerTypeInteger,
			SkillID:    skill.ID,
			Tier:       skillgraph.TierLearn,
		}
		HandleAnswer(state, "wrong")
	}

	errors := state.RecentErrors[skill.ID]
	if len(errors) != MaxRecentErrors {
		t.Errorf("RecentErrors length = %d, want %d", len(errors), MaxRecentErrors)
	}
}

func TestBuildSummary(t *testing.T) {
	state := testState()
	state.TotalQuestions = 14
	state.TotalCorrect = 11
	state.Elapsed = 15 * time.Minute

	skill := state.Plan.Slots[0].Skill
	sr := state.PerSkillResults[skill.ID]
	sr.Attempted = 6
	sr.Correct = 5

	summary := BuildSummary(state)

	if summary.TotalQuestions != 14 {
		t.Errorf("TotalQuestions = %d, want 14", summary.TotalQuestions)
	}
	if summary.TotalCorrect != 11 {
		t.Errorf("TotalCorrect = %d, want 11", summary.TotalCorrect)
	}
	if summary.Accuracy < 0.78 || summary.Accuracy > 0.79 {
		t.Errorf("Accuracy = %f, want ~0.785", summary.Accuracy)
	}
	if len(summary.SkillResults) == 0 {
		t.Error("expected non-empty SkillResults")
	}
}

func TestCurrentSlot(t *testing.T) {
	state := testState()

	slot := CurrentSlot(state)
	if slot == nil {
		t.Fatal("expected non-nil slot")
	}
	if slot.Skill.ID != state.Plan.Slots[0].Skill.ID {
		t.Errorf("slot skill = %s, want %s", slot.Skill.ID, state.Plan.Slots[0].Skill.ID)
	}
}

func TestNewSessionState(t *testing.T) {
	plan := testPlan()
	state := NewSessionState(plan, "test-id", nil, nil)

	if state.SessionID != "test-id" {
		t.Errorf("SessionID = %q, want %q", state.SessionID, "test-id")
	}
	if state.Phase != PhaseActive {
		t.Errorf("Phase = %d, want PhaseActive", state.Phase)
	}
	if len(state.PerSkillResults) == 0 {
		t.Error("expected PerSkillResults to be populated")
	}
	if state.Mastered == nil {
		t.Error("expected Mastered map to be initialized")
	}
	if state.TierProgress == nil {
		t.Error("expected TierProgress map to be initialized")
	}
}
