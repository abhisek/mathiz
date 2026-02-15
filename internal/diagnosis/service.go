package diagnosis

import (
	"context"

	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/skillgraph"
)

// Service coordinates error diagnosis using rule-based classifiers and
// optional LLM-based misconception identification.
type Service struct {
	classifiers []Classifier
	diagnoser   *Diagnoser
	pending     chan diagnosisJob
}

type diagnosisJob struct {
	ctx context.Context
	req *DiagnosisRequest
	cb  func(*DiagnosisResult)
}

// NewService creates a diagnosis service. If provider is nil, only rule-based
// classification is available.
func NewService(provider llm.Provider) *Service {
	s := &Service{
		classifiers: DefaultClassifiers(),
		pending:     make(chan diagnosisJob, 32),
	}
	if provider != nil {
		s.diagnoser = NewDiagnoser(provider, DefaultDiagnoserConfig())
		go s.processLoop()
	}
	return s
}

// Diagnose classifies a wrong answer. Rule-based classification is synchronous.
// If rules are inconclusive and an LLM is available, async LLM diagnosis is
// dispatched and the callback fires when the result is ready.
// Returns the synchronous result immediately.
func (s *Service) Diagnose(
	ctx context.Context,
	question *problemgen.Question,
	learnerAnswer string,
	responseTimeMs int,
	skillAccuracy float64,
	cb func(*DiagnosisResult),
) *DiagnosisResult {
	input := &ClassifyInput{
		Question:       question,
		LearnerAnswer:  learnerAnswer,
		ResponseTimeMs: responseTimeMs,
		SkillAccuracy:  skillAccuracy,
	}

	// Phase 1: Rule-based (synchronous).
	cat, conf, name := RunClassifiers(s.classifiers, input)
	if cat != "" {
		return &DiagnosisResult{
			Category:       cat,
			Confidence:     conf,
			ClassifierName: name,
		}
	}

	// Phase 2: LLM (async).
	if s.diagnoser != nil {
		s.dispatchLLM(ctx, question, learnerAnswer, cb)
	}

	// Return unclassified immediately; LLM result arrives via callback.
	return &DiagnosisResult{
		Category:       CategoryUnclassified,
		Confidence:     0,
		ClassifierName: "none",
	}
}

func (s *Service) dispatchLLM(
	ctx context.Context,
	q *problemgen.Question,
	learnerAnswer string,
	cb func(*DiagnosisResult),
) {
	skill, err := skillgraph.GetSkill(q.SkillID)
	if err != nil {
		return
	}

	candidates := MisconceptionsByStrand(skill.Strand)
	if len(candidates) == 0 {
		return
	}

	req := &DiagnosisRequest{
		SkillID:       q.SkillID,
		SkillName:     skill.Name,
		QuestionText:  q.Text,
		CorrectAnswer: q.Answer,
		LearnerAnswer: learnerAnswer,
		AnswerType:    string(q.AnswerType),
		Candidates:    candidates,
	}

	select {
	case s.pending <- diagnosisJob{ctx: ctx, req: req, cb: cb}:
	default:
		// Channel full — drop diagnosis silently. Not critical.
	}
}

func (s *Service) processLoop() {
	for job := range s.pending {
		result, err := s.diagnoser.Diagnose(job.ctx, job.req)
		if err != nil || result == nil {
			continue
		}
		if job.cb != nil {
			job.cb(result)
		}
	}
}

// Close shuts down the async processing loop.
func (s *Service) Close() {
	close(s.pending)
}
