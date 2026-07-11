package store

import (
	"context"
	gosql "database/sql"
	"fmt"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/llmrequestevent"
)

// eventRepo implements EventRepo backed by ent and the global sequence counter.
// All reads and writes are scoped to a single owner (learner): every append
// stamps the owner and every query filters by it, so learners sharing one
// database (SaaS mode) are fully isolated.
type eventRepo struct {
	client  *ent.Client
	seq     *sequenceCounter
	db      *gosql.DB
	dialect string
	owner   string
}

// scope stamps the repo's owner into ctx so the store-level owner guard
// (see ownerguard.go) scopes every ent call made during the request. Every
// exported repo method must wrap its ctx with this at entry.
func (r *eventRepo) scope(ctx context.Context) context.Context {
	return withOwner(ctx, r.owner)
}

// ownerPlaceholder returns the SQL bind placeholder for the owner parameter
// in raw (non-ent) queries, per dialect.
func (r *eventRepo) ownerPlaceholder() string {
	if r.dialect == dialect.Postgres {
		return "$1"
	}
	return "?"
}

func (r *eventRepo) AppendLLMRequest(ctx context.Context, data LLMRequestEventData) error {
	ctx = r.scope(ctx)
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	_, err = r.client.LLMRequestEvent.Create().
		SetSequence(seqNum).
		SetOwnerID(r.owner).
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
	ctx = r.scope(ctx)
	q := r.client.LLMRequestEvent.Query().
		Where(llmrequestevent.OwnerID(r.owner))

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
	ctx = r.scope(ctx)
	row, err := r.client.LLMRequestEvent.Query().
		Where(llmrequestevent.ID(id), llmrequestevent.OwnerID(r.owner)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get LLM event %d: %w", id, err)
	}
	rec := llmEventToRecord(row)
	return &rec, nil
}

func (r *eventRepo) LLMUsageByPurpose(ctx context.Context) ([]LLMUsageStats, error) {
	ctx = r.scope(ctx) // raw SQL bypasses the guard; parameterizes owner explicitly below
	rows, err := r.db.QueryContext(ctx, `
		SELECT purpose,
		       COUNT(*) as calls,
		       COALESCE(SUM(input_tokens), 0) as input_tokens,
		       COALESCE(SUM(output_tokens), 0) as output_tokens,
		       CAST(COALESCE(AVG(latency_ms), 0) AS BIGINT) as avg_latency
		FROM llm_request_events
		WHERE owner_id = `+r.ownerPlaceholder()+`
		GROUP BY purpose
		ORDER BY calls DESC`, r.owner)
	if err != nil {
		return nil, fmt.Errorf("query LLM usage: %w", err)
	}
	defer rows.Close()

	var stats []LLMUsageStats
	for rows.Next() {
		var s LLMUsageStats
		if err := rows.Scan(&s.Purpose, &s.Calls, &s.InputTokens, &s.OutputTokens, &s.AvgLatencyMs); err != nil {
			return nil, fmt.Errorf("scan LLM usage row: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

func (r *eventRepo) LLMUsageByModel(ctx context.Context) ([]LLMModelUsage, error) {
	ctx = r.scope(ctx) // raw SQL bypasses the guard; parameterizes owner explicitly below
	rows, err := r.db.QueryContext(ctx, `
		SELECT model,
		       COUNT(*) as calls,
		       COALESCE(SUM(input_tokens), 0) as input_tokens,
		       COALESCE(SUM(output_tokens), 0) as output_tokens
		FROM llm_request_events
		WHERE owner_id = `+r.ownerPlaceholder()+`
		GROUP BY model
		ORDER BY calls DESC`, r.owner)
	if err != nil {
		return nil, fmt.Errorf("query LLM usage by model: %w", err)
	}
	defer rows.Close()

	var usage []LLMModelUsage
	for rows.Next() {
		var u LLMModelUsage
		if err := rows.Scan(&u.Model, &u.Calls, &u.InputTokens, &u.OutputTokens); err != nil {
			return nil, fmt.Errorf("scan LLM model usage row: %w", err)
		}
		usage = append(usage, u)
	}
	return usage, rows.Err()
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
