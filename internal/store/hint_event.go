package store

import (
	"context"
	"fmt"

	"github.com/abhisek/mathiz/ent/hintevent"
)

func (r *eventRepo) AppendHintEvent(ctx context.Context, data HintEventData) error {
	ctx = r.scope(ctx)
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	_, err = r.client.HintEvent.Create().
		SetSequence(seqNum).
		SetOwnerID(r.owner).
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

func (r *eventRepo) HintCountForSession(ctx context.Context, sessionID string) (int, error) {
	ctx = r.scope(ctx)
	n, err := r.client.HintEvent.Query().
		Where(hintevent.OwnerID(r.owner), hintevent.SessionID(sessionID)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count session hints: %w", err)
	}
	return n, nil
}
