package store

import (
	"context"
	"fmt"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/answerevent"
	"github.com/abhisek/mathiz/ent/masteryevent"
)

func (r *eventRepo) AppendMasteryEvent(ctx context.Context, data MasteryEventData) error {
	ctx = r.scope(ctx)
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	builder := r.client.MasteryEvent.Create().
		SetSequence(seqNum).
		SetOwnerID(r.owner).
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

func (r *eventRepo) QueryMasteryEvents(ctx context.Context, opts QueryOpts) ([]MasteryEventRecord, error) {
	ctx = r.scope(ctx)
	query := r.client.MasteryEvent.Query().
		Where(masteryevent.OwnerID(r.owner)).
		Order(ent.Desc(masteryevent.FieldSequence))

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.After > 0 {
		query = query.Where(masteryevent.SequenceGT(opts.After))
	}
	if opts.Before > 0 {
		query = query.Where(masteryevent.SequenceLT(opts.Before))
	}
	if !opts.From.IsZero() {
		query = query.Where(masteryevent.TimestampGTE(opts.From))
	}
	if !opts.To.IsZero() {
		query = query.Where(masteryevent.TimestampLTE(opts.To))
	}

	events, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query mastery events: %w", err)
	}

	records := make([]MasteryEventRecord, len(events))
	for i, e := range events {
		records[i] = MasteryEventRecord{
			Sequence:     e.Sequence,
			Timestamp:    e.Timestamp,
			SkillID:      e.SkillID,
			FromState:    e.FromState,
			ToState:      e.ToState,
			Trigger:      e.Trigger,
			FluencyScore: e.FluencyScore,
			SessionID:    e.SessionID,
		}
	}
	return records, nil
}

func (r *eventRepo) RecentReviewAccuracy(ctx context.Context, skillID string, lastN int) (float64, int, error) {
	ctx = r.scope(ctx)
	events, err := r.client.AnswerEvent.Query().
		Where(
			answerevent.OwnerID(r.owner),
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
