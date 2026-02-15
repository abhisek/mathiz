package lessons

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/skillgraph"
)

func validLessonJSON() json.RawMessage {
	return json.RawMessage(`{
		"title": "Carrying in Addition",
		"explanation": "When you add numbers and the sum in a column is 10 or more, you carry to the next column.",
		"worked_example": "1. Add ones: 7 + 5 = 12\n2. Write 2, carry 1\n3. Add tens: 4 + 3 + 1 = 8\nAnswer: 82",
		"practice_question": {
			"text": "What is 27 + 15?",
			"answer": "42",
			"answer_type": "integer",
			"explanation": "7 + 5 = 12, write 2 carry 1. 2 + 1 + 1 = 4. Answer: 42"
		}
	}`)
}

func testSkill() skillgraph.Skill {
	skills := skillgraph.AllSkills()
	if len(skills) == 0 {
		panic("no skills in graph")
	}
	return skills[0]
}

func TestService_GeneratesLesson(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{
		Content: validLessonJSON(),
	})
	svc := NewService(mock, DefaultConfig())

	input := LessonInput{
		Skill:    testSkill(),
		Accuracy: 0.4,
		RecentErrors: []string{
			"Answered 715 for '47 + 38', correct was 85",
		},
	}

	svc.RequestLesson(t.Context(), input)

	// Poll for result.
	var lesson *Lesson
	var ok bool
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		lesson, ok = svc.ConsumeLesson()
		if ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !ok || lesson == nil {
		t.Fatal("expected lesson to be generated")
	}

	if lesson.Title != "Carrying in Addition" {
		t.Errorf("expected title 'Carrying in Addition', got %q", lesson.Title)
	}
	if lesson.Explanation == "" {
		t.Error("expected non-empty explanation")
	}
	if lesson.WorkedExample == "" {
		t.Error("expected non-empty worked example")
	}
	if lesson.PracticeQuestion.Text == "" {
		t.Error("expected non-empty practice question text")
	}
	if lesson.PracticeQuestion.Answer != "42" {
		t.Errorf("expected practice answer '42', got %q", lesson.PracticeQuestion.Answer)
	}
	if lesson.PracticeQuestion.AnswerType != "integer" {
		t.Errorf("expected answer type 'integer', got %q", lesson.PracticeQuestion.AnswerType)
	}
}

func TestService_ConsumeClearsLesson(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{
		Content: validLessonJSON(),
	})
	svc := NewService(mock, DefaultConfig())

	svc.RequestLesson(t.Context(), LessonInput{Skill: testSkill()})

	// Wait for generation.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := svc.ConsumeLesson(); ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Second consume should return false.
	_, ok := svc.ConsumeLesson()
	if ok {
		t.Error("expected second ConsumeLesson to return false")
	}
}

func TestService_LLMError(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{
		Err: &llm.ErrProviderUnavailable{},
	})
	svc := NewService(mock, DefaultConfig())

	svc.RequestLesson(t.Context(), LessonInput{Skill: testSkill()})

	// Wait a bit for async completion.
	time.Sleep(100 * time.Millisecond)

	// Should not have a lesson.
	lesson, ok := svc.ConsumeLesson()
	if ok && lesson != nil {
		t.Error("expected no lesson on LLM error")
	}
}

func TestService_PurposeLabel(t *testing.T) {
	mock := llm.NewMockProvider(llm.MockResponse{
		Content: validLessonJSON(),
	})
	svc := NewService(mock, DefaultConfig())

	svc.RequestLesson(t.Context(), LessonInput{Skill: testSkill()})

	// Wait for generation.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := svc.ConsumeLesson(); ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if mock.CallCount() != 1 {
		t.Fatalf("expected 1 call, got %d", mock.CallCount())
	}

	// Check that the schema was set.
	req := mock.Calls[0]
	if req.Schema == nil || req.Schema.Name != "micro-lesson" {
		t.Error("expected schema name 'micro-lesson'")
	}
}
