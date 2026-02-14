package store

import (
	"context"
	"fmt"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/answerevent"
)

func (r *eventRepo) AppendMasteryEvent(ctx context.Context, data MasteryEventData) error {
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	builder := r.client.MasteryEvent.Create().
		SetSequence(seqNum).
		SetSkillID(data.SkillID).
		SetFromState(data.FromState).
		SetToState(data.ToState).
		SetTrigger(data.Trigger).
		SetFluencyScore(data.FluencyScore)

	if data.SessionID != "" {
		builder = builder.SetSessionID(data.SessionID)
	}

	_, err = builder.Save(ctx)
	if err != nil {
		return fmt.Errorf("save mastery event: %w", err)
	}
	return nil
}

func (r *eventRepo) RecentReviewAccuracy(ctx context.Context, skillID string, lastN int) (float64, int, error) {
	events, err := r.client.AnswerEvent.Query().
		Where(
			answerevent.SkillID(skillID),
			answerevent.Category("review"),
		).
		Order(ent.Desc(answerevent.FieldSequence)).
		Limit(lastN).
		All(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("query review answers: %w", err)
	}

	count := len(events)
	if count == 0 {
		return 0, 0, nil
	}

	correct := 0
	for _, e := range events {
		if e.Correct {
			correct++
		}
	}

	return float64(correct) / float64(count), count, nil
}
