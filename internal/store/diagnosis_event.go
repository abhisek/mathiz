package store

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/diagnosisevent"
)

func (r *eventRepo) AppendDiagnosisEvent(ctx context.Context, data DiagnosisEventData) error {
	ctx = r.scope(ctx)
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	builder := r.client.DiagnosisEvent.Create().
		SetSequence(seqNum).
		SetOwnerID(r.owner).
		SetSessionID(data.SessionID).
		SetSkillID(data.SkillID).
		SetQuestionText(data.QuestionText).
		SetCorrectAnswer(data.CorrectAnswer).
		SetLearnerAnswer(data.LearnerAnswer).
		SetCategory(data.Category).
		SetConfidence(data.Confidence).
		SetClassifierName(data.ClassifierName).
		SetReasoning(data.Reasoning)

	if data.MisconceptionID != nil {
		builder = builder.SetMisconceptionID(*data.MisconceptionID)
	}

	_, err = builder.Save(ctx)
	if err != nil {
		return fmt.Errorf("save diagnosis event: %w", err)
	}
	return nil
}

// DiagnosisCounts aggregates in SQL: a child's diagnosis history grows with
// every wrong answer, and only the per-pattern totals are ever needed.
func (r *eventRepo) DiagnosisCounts(ctx context.Context, opts QueryOpts) ([]DiagnosisCount, error) {
	ctx = r.scope(ctx)
	query := r.client.DiagnosisEvent.Query().Where(diagnosisevent.OwnerID(r.owner))
	if !opts.From.IsZero() {
		query = query.Where(diagnosisevent.TimestampGTE(opts.From))
	}
	if !opts.To.IsZero() {
		query = query.Where(diagnosisevent.TimestampLTE(opts.To))
	}

	var rows []struct {
		SkillID         string  `json:"skill_id"`
		Category        string  `json:"category"`
		MisconceptionID *string `json:"misconception_id"`
		Count           int     `json:"count"`
	}
	err := query.
		GroupBy(diagnosisevent.FieldSkillID, diagnosisevent.FieldCategory, diagnosisevent.FieldMisconceptionID).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("count diagnoses: %w", err)
	}

	out := make([]DiagnosisCount, len(rows))
	for i, row := range rows {
		out[i] = DiagnosisCount{SkillID: row.SkillID, Category: row.Category, Count: row.Count}
		if row.MisconceptionID != nil {
			out[i].MisconceptionID = *row.MisconceptionID
		}
	}
	slices.SortFunc(out, func(a, b DiagnosisCount) int {
		return cmp.Or(
			cmp.Compare(b.Count, a.Count),
			cmp.Compare(a.SkillID, b.SkillID),
			cmp.Compare(a.Category, b.Category),
			cmp.Compare(a.MisconceptionID, b.MisconceptionID),
		)
	})
	return out, nil
}
