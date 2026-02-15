package store

import (
	"context"
	"fmt"
)

func (r *eventRepo) AppendLessonEvent(ctx context.Context, data LessonEventData) error {
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	_, err = r.client.LessonEvent.Create().
		SetSequence(seqNum).
		SetSessionID(data.SessionID).
		SetSkillID(data.SkillID).
		SetLessonTitle(data.LessonTitle).
		SetPracticeAttempted(data.PracticeAttempted).
		SetPracticeCorrect(data.PracticeCorrect).
		SetPracticeSkipped(data.PracticeSkipped).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("save lesson event: %w", err)
	}
	return nil
}
