package store

import (
	"context"
	"fmt"
)

func (r *eventRepo) AppendHintEvent(ctx context.Context, data HintEventData) error {
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	_, err = r.client.HintEvent.Create().
		SetSequence(seqNum).
		SetSessionID(data.SessionID).
		SetSkillID(data.SkillID).
		SetQuestionText(data.QuestionText).
		SetHintText(data.HintText).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("save hint event: %w", err)
	}
	return nil
}
