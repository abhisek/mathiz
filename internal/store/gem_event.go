package store

import (
	"context"
	"fmt"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/gemevent"
	"github.com/abhisek/mathiz/ent/sessionevent"
)

func (r *eventRepo) AppendGemEvent(ctx context.Context, data GemEventData) error {
	ctx = r.scope(ctx)
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	builder := r.client.GemEvent.Create().
		SetSequence(seqNum).
		SetOwnerID(r.owner).
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
	ctx = r.scope(ctx)
	query := r.client.GemEvent.Query().
		Where(gemevent.OwnerID(r.owner)).
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
	ctx = r.scope(ctx)
	// Aggregate in SQL: loading every gem row just to count grows without
	// bound as a learner accumulates history.
	var rows []struct {
		GemType string `json:"gem_type"`
		Count   int    `json:"count"`
	}
	err := r.client.GemEvent.Query().
		Where(gemevent.OwnerID(r.owner)).
		GroupBy(gemevent.FieldGemType).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, 0, fmt.Errorf("query gem counts: %w", err)
	}

	byType := make(map[string]int, len(rows))
	total := 0
	for _, row := range rows {
		byType[row.GemType] = row.Count
		total += row.Count
	}
	return byType, total, nil
}

func (r *eventRepo) QuerySessionSummaries(ctx context.Context, opts QueryOpts) ([]SessionSummaryRecord, error) {
	ctx = r.scope(ctx)
	query := r.client.SessionEvent.Query().
		Where(sessionevent.OwnerID(r.owner), sessionevent.Action("end")).
		Order(ent.Desc(sessionevent.FieldSequence))

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.After > 0 {
		query = query.Where(sessionevent.SequenceGT(opts.After))
	}
	if opts.Before > 0 {
		query = query.Where(sessionevent.SequenceLT(opts.Before))
	}
	if !opts.From.IsZero() {
		query = query.Where(sessionevent.TimestampGTE(opts.From))
	}
	if !opts.To.IsZero() {
		query = query.Where(sessionevent.TimestampLTE(opts.To))
	}

	events, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query session summaries: %w", err)
	}
	if len(events) == 0 {
		return nil, nil
	}

	// One grouped query for all sessions' gem counts instead of N counts.
	sessionIDs := make([]string, len(events))
	for i, e := range events {
		sessionIDs[i] = e.SessionID
	}
	var gemRows []struct {
		SessionID string `json:"session_id"`
		Count     int    `json:"count"`
	}
	err = r.client.GemEvent.Query().
		Where(gemevent.OwnerID(r.owner), gemevent.SessionIDIn(sessionIDs...)).
		GroupBy(gemevent.FieldSessionID).
		Aggregate(ent.Count()).
		Scan(ctx, &gemRows)
	if err != nil {
		return nil, fmt.Errorf("count session gems: %w", err)
	}
	gemsBySession := make(map[string]int, len(gemRows))
	for _, row := range gemRows {
		gemsBySession[row.SessionID] = row.Count
	}

	// One query for all matching "start" events joins each summary with the
	// plan it opened with (instead of N per-session lookups).
	starts, err := r.client.SessionEvent.Query().
		Where(
			sessionevent.OwnerID(r.owner),
			sessionevent.Action("start"),
			sessionevent.SessionIDIn(sessionIDs...),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query session starts: %w", err)
	}
	planBySession := make(map[string][]PlanSlotSummaryData, len(starts))
	for _, st := range starts {
		if len(st.PlanSummary) == 0 {
			continue
		}
		plan := make([]PlanSlotSummaryData, len(st.PlanSummary))
		for i, slot := range st.PlanSummary {
			plan[i] = PlanSlotSummaryData{
				SkillID:  slot.SkillID,
				Tier:     slot.Tier,
				Category: slot.Category,
			}
		}
		planBySession[st.SessionID] = plan
	}

	records := make([]SessionSummaryRecord, len(events))
	for i, e := range events {
		records[i] = SessionSummaryRecord{
			SessionID:       e.SessionID,
			Sequence:        e.Sequence,
			Timestamp:       e.Timestamp,
			QuestionsServed: e.QuestionsServed,
			CorrectAnswers:  e.CorrectAnswers,
			DurationSecs:    e.DurationSecs,
			GemCount:        gemsBySession[e.SessionID],
			Plan:            planBySession[e.SessionID],
		}
	}
	return records, nil
}
