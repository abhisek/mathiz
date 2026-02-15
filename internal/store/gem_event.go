package store

import (
	"context"
	"fmt"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/gemevent"
	"github.com/abhisek/mathiz/ent/sessionevent"
)

func (r *eventRepo) AppendGemEvent(ctx context.Context, data GemEventData) error {
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	builder := r.client.GemEvent.Create().
		SetSequence(seqNum).
		SetGemType(data.GemType).
		SetRarity(data.Rarity).
		SetSessionID(data.SessionID).
		SetReason(data.Reason)

	if data.SkillID != nil {
		builder = builder.SetSkillID(*data.SkillID)
	}
	if data.SkillName != nil {
		builder = builder.SetSkillName(*data.SkillName)
	}

	_, err = builder.Save(ctx)
	if err != nil {
		return fmt.Errorf("save gem event: %w", err)
	}
	return nil
}

func (r *eventRepo) QueryGemEvents(ctx context.Context, opts QueryOpts) ([]GemEventRecord, error) {
	query := r.client.GemEvent.Query().
		Order(ent.Desc(gemevent.FieldSequence))

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.After > 0 {
		query = query.Where(gemevent.SequenceGT(opts.After))
	}
	if opts.Before > 0 {
		query = query.Where(gemevent.SequenceLT(opts.Before))
	}
	if !opts.From.IsZero() {
		query = query.Where(gemevent.TimestampGTE(opts.From))
	}
	if !opts.To.IsZero() {
		query = query.Where(gemevent.TimestampLTE(opts.To))
	}

	events, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query gem events: %w", err)
	}

	records := make([]GemEventRecord, len(events))
	for i, e := range events {
		records[i] = GemEventRecord{
			GemType:   e.GemType,
			Rarity:    e.Rarity,
			SkillID:   e.SkillID,
			SkillName: e.SkillName,
			SessionID: e.SessionID,
			Reason:    e.Reason,
			Sequence:  e.Sequence,
			Timestamp: e.Timestamp,
		}
	}
	return records, nil
}

func (r *eventRepo) GemCounts(ctx context.Context) (map[string]int, int, error) {
	events, err := r.client.GemEvent.Query().All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query gem counts: %w", err)
	}

	byType := make(map[string]int)
	for _, e := range events {
		byType[e.GemType]++
	}

	total := len(events)
	return byType, total, nil
}

func (r *eventRepo) QuerySessionSummaries(ctx context.Context, opts QueryOpts) ([]SessionSummaryRecord, error) {
	query := r.client.SessionEvent.Query().
		Where(sessionevent.Action("end")).
		Order(ent.Desc(sessionevent.FieldSequence))

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}

	events, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query session summaries: %w", err)
	}

	records := make([]SessionSummaryRecord, len(events))
	for i, e := range events {
		// Count gems for this session.
		gemCount, _ := r.client.GemEvent.Query().
			Where(gemevent.SessionID(e.SessionID)).
			Count(ctx)

		records[i] = SessionSummaryRecord{
			SessionID:       e.SessionID,
			Timestamp:       e.Timestamp,
			QuestionsServed: e.QuestionsServed,
			CorrectAnswers:  e.CorrectAnswers,
			DurationSecs:    e.DurationSecs,
			GemCount:        gemCount,
		}
	}
	return records, nil
}
