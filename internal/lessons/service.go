package lessons

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/abhisek/mathiz/internal/llm"
)

// Service generates micro-lessons asynchronously.
type Service struct {
	provider llm.Provider
	cfg      Config

	mu      sync.Mutex
	pending *Lesson
	err     error
	ready   bool
}

// NewService creates a lesson generation service.
func NewService(provider llm.Provider, cfg Config) *Service {
	return &Service{provider: provider, cfg: cfg}
}

// RequestLesson starts async lesson generation. Only one lesson is in-flight
// at a time — new requests replace pending ones.
func (s *Service) RequestLesson(ctx context.Context, input LessonInput) {
	go func() {
		lesson, err := s.generate(ctx, input)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.pending = lesson
		s.err = err
		s.ready = true
	}()
}

// ConsumeLesson returns the pending lesson if one is ready.
// Returns (nil, false) if no lesson is ready yet.
// After consumption, the pending slot is cleared.
func (s *Service) ConsumeLesson() (*Lesson, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready {
		return nil, false
	}
	lesson := s.pending
	s.pending = nil
	s.ready = false
	s.err = nil
	return lesson, lesson != nil
}

type lessonOutput struct {
	Title            string                 `json:"title"`
	Explanation      string                 `json:"explanation"`
	WorkedExample    string                 `json:"worked_example"`
	PracticeQuestion practiceQuestionOutput `json:"practice_question"`
}

type practiceQuestionOutput struct {
	Text        string `json:"text"`
	Answer      string `json:"answer"`
	AnswerType  string `json:"answer_type"`
	Explanation string `json:"explanation"`
}

func (s *Service) generate(ctx context.Context, input LessonInput) (*Lesson, error) {
	ctx = llm.WithPurpose(ctx, "lesson")

	userMsg := buildLessonUserMessage(input)

	req := llm.Request{
		System: lessonSystemPrompt,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: userMsg},
		},
		Schema:      LessonSchema,
		MaxTokens:   s.cfg.MaxTokens,
		Temperature: s.cfg.Temperature,
	}

	resp, err := s.provider.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("lesson generation: %w", err)
	}

	var out lessonOutput
	if err := json.Unmarshal(resp.Content, &out); err != nil {
		return nil, fmt.Errorf("parse lesson response: %w", err)
	}

	return &Lesson{
		SkillID:       input.Skill.ID,
		Title:         out.Title,
		Explanation:   out.Explanation,
		WorkedExample: out.WorkedExample,
		PracticeQuestion: PracticeQuestion{
			Text:        out.PracticeQuestion.Text,
			Answer:      out.PracticeQuestion.Answer,
			AnswerType:  out.PracticeQuestion.AnswerType,
			Explanation: out.PracticeQuestion.Explanation,
		},
	}, nil
}
