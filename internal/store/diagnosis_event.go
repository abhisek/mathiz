package store

import (
	"context"
	"fmt"
)

func (r *eventRepo) AppendDiagnosisEvent(ctx context.Context, data DiagnosisEventData) error {
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	builder := r.client.DiagnosisEvent.Create().
		SetSequence(seqNum).
		SetSessionID(data.SessionID).
		SetSkillID(data.SkillID).
		SetQuestionText(data.QuestionText).
		SetCorrectAnswer(data.CorrectAnswer).
		SetLearnerAnswer(data.LearnerAnswer).
		SetCategory(data.Category).
		SetConfidence(data.Confidence).
		SetClassifierName(data.ClassifierName).
		SetReasoning(data.Reasoning)

	if data.MisconceptionID != nil {
		builder = builder.SetMisconceptionID(*data.MisconceptionID)
	}

	_, err = builder.Save(ctx)
	if err != nil {
		return fmt.Errorf("save diagnosis event: %w", err)
	}
	return nil
}
