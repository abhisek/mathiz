package store

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/llmrequestevent"
)

// eventRepo implements EventRepo backed by ent and the global sequence counter.
type eventRepo struct {
	client *ent.Client
	seq    *sequenceCounter
}

func (r *eventRepo) AppendLLMRequest(ctx context.Context, data LLMRequestEventData) error {
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	_, err = r.client.LLMRequestEvent.Create().
		SetSequence(seqNum).
		SetProvider(data.Provider).
		SetModel(data.Model).
		SetPurpose(data.Purpose).
		SetInputTokens(data.InputTokens).
		SetOutputTokens(data.OutputTokens).
		SetLatencyMs(data.LatencyMs).
		SetSuccess(data.Success).
		SetErrorMessage(data.ErrorMessage).
		SetRequestBody(data.RequestBody).
		SetResponseBody(data.ResponseBody).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("save LLM request event: %w", err)
	}

	return nil
}

func (r *eventRepo) QueryLLMEvents(ctx context.Context, opts QueryOpts) ([]LLMRequestEventRecord, error) {
	q := r.client.LLMRequestEvent.Query()

	if !opts.From.IsZero() {
		q = q.Where(llmrequestevent.TimestampGTE(opts.From))
	}
	if !opts.To.IsZero() {
		q = q.Where(llmrequestevent.TimestampLTE(opts.To))
	}

	q = q.Order(llmrequestevent.ByID(sql.OrderDesc()))

	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	} else {
		q = q.Limit(50)
	}

	rows, err := q.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query LLM events: %w", err)
	}

	records := make([]LLMRequestEventRecord, len(rows))
	for i, row := range rows {
		records[i] = llmEventToRecord(row)
	}
	return records, nil
}

func (r *eventRepo) GetLLMEvent(ctx context.Context, id int) (*LLMRequestEventRecord, error) {
	row, err := r.client.LLMRequestEvent.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get LLM event %d: %w", id, err)
	}
	rec := llmEventToRecord(row)
	return &rec, nil
}

func llmEventToRecord(row *ent.LLMRequestEvent) LLMRequestEventRecord {
	return LLMRequestEventRecord{
		ID:           row.ID,
		Sequence:     row.Sequence,
		Timestamp:    row.Timestamp,
		Provider:     row.Provider,
		Model:        row.Model,
		Purpose:      row.Purpose,
		InputTokens:  row.InputTokens,
		OutputTokens: row.OutputTokens,
		LatencyMs:    row.LatencyMs,
		Success:      row.Success,
		ErrorMessage: row.ErrorMessage,
		RequestBody:  row.RequestBody,
		ResponseBody: row.ResponseBody,
	}
}
