package store

import (
	"context"
	"fmt"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/learnerprofileevent"
)

func (r *eventRepo) AppendLearnerProfileEvent(ctx context.Context, data LearnerProfileEventData) error {
	ctx = r.scope(ctx)
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	_, err = r.client.LearnerProfileEvent.Create().
		SetSequence(seqNum).
		SetOwnerID(r.owner).
		SetSummary(data.Summary).
		SetStrengths(data.Strengths).
		SetWeaknesses(data.Weaknesses).
		SetPatterns(data.Patterns).
		SetGeneratedAt(data.GeneratedAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("save learner profile event: %w", err)
	}
	return nil
}

func (r *eventRepo) QueryLearnerProfileEvents(ctx context.Context, opts QueryOpts) ([]LearnerProfileEventRecord, error) {
	ctx = r.scope(ctx)
	query := r.client.LearnerProfileEvent.Query().
		Where(learnerprofileevent.OwnerID(r.owner)).
		Order(ent.Desc(learnerprofileevent.FieldSequence))

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.After > 0 {
		query = query.Where(learnerprofileevent.SequenceGT(opts.After))
	}
	if opts.Before > 0 {
		query = query.Where(learnerprofileevent.SequenceLT(opts.Before))
	}
	if !opts.From.IsZero() {
		query = query.Where(learnerprofileevent.TimestampGTE(opts.From))
	}
	if !opts.To.IsZero() {
		query = query.Where(learnerprofileevent.TimestampLTE(opts.To))
	}

	events, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query learner profile events: %w", err)
	}

	records := make([]LearnerProfileEventRecord, len(events))
	for i, e := range events {
		records[i] = LearnerProfileEventRecord{
			Sequence:    e.Sequence,
			Timestamp:   e.Timestamp,
			Summary:     e.Summary,
			Strengths:   e.Strengths,
			Weaknesses:  e.Weaknesses,
			Patterns:    e.Patterns,
			GeneratedAt: e.GeneratedAt,
		}
	}
	return records, nil
}
