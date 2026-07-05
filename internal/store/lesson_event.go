package store

import (
	"context"
	"fmt"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/lessonevent"
)

func (r *eventRepo) AppendLessonEvent(ctx context.Context, data LessonEventData) error {
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	_, err = r.client.LessonEvent.Create().
		SetSequence(seqNum).
		SetOwnerID(r.owner).
		SetSessionID(data.SessionID).
		SetSkillID(data.SkillID).
		SetLessonTitle(data.LessonTitle).
		SetPracticeAttempted(data.PracticeAttempted).
		SetPracticeCorrect(data.PracticeCorrect).
		SetPracticeSkipped(data.PracticeSkipped).
		SetExplanation(data.Explanation).
		SetWorkedExample(data.WorkedExample).
		SetPracticeText(data.PracticeText).
		SetPracticeAnswer(data.PracticeAnswer).
		SetPracticeExplanation(data.PracticeExplanation).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("save lesson event: %w", err)
	}
	return nil
}

func (r *eventRepo) QueryLessonEvents(ctx context.Context, opts QueryOpts) ([]LessonEventRecord, error) {
	query := r.client.LessonEvent.Query().
		Where(lessonevent.OwnerID(r.owner)).
		Order(ent.Desc(lessonevent.FieldSequence))

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.After > 0 {
		query = query.Where(lessonevent.SequenceGT(opts.After))
	}
	if opts.Before > 0 {
		query = query.Where(lessonevent.SequenceLT(opts.Before))
	}
	if !opts.From.IsZero() {
		query = query.Where(lessonevent.TimestampGTE(opts.From))
	}
	if !opts.To.IsZero() {
		query = query.Where(lessonevent.TimestampLTE(opts.To))
	}

	events, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query lesson events: %w", err)
	}

	records := make([]LessonEventRecord, len(events))
	for i, e := range events {
		records[i] = LessonEventRecord{
			Sequence:            e.Sequence,
			Timestamp:           e.Timestamp,
			SessionID:           e.SessionID,
			SkillID:             e.SkillID,
			LessonTitle:         e.LessonTitle,
			Explanation:         e.Explanation,
			WorkedExample:       e.WorkedExample,
			PracticeText:        e.PracticeText,
			PracticeAnswer:      e.PracticeAnswer,
			PracticeExplanation: e.PracticeExplanation,
			PracticeAttempted:   e.PracticeAttempted,
			PracticeCorrect:     e.PracticeCorrect,
			PracticeSkipped:     e.PracticeSkipped,
		}
	}
	return records, nil
}
