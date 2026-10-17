package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/answerevent"
	entschema "github.com/abhisek/mathiz/ent/schema"
	"github.com/abhisek/mathiz/ent/sessionevent"
)

func (r *eventRepo) AppendSessionEvent(ctx context.Context, data SessionEventData) error {
	ctx = r.scope(ctx)
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	var planSummary []entschema.PlanSlotSummary
	for _, s := range data.PlanSummary {
		planSummary = append(planSummary, entschema.PlanSlotSummary{
			SkillID:  s.SkillID,
			Tier:     s.Tier,
			Category: s.Category,
		})
	}

	builder := r.client.SessionEvent.Create().
		SetSequence(seqNum).
		SetOwnerID(r.owner).
		SetSessionID(data.SessionID).
		SetAction(data.Action).
		SetQuestionsServed(data.QuestionsServed).
		SetCorrectAnswers(data.CorrectAnswers).
		SetDurationSecs(data.DurationSecs).
		SetQuestUID(data.QuestUID).
		SetQuestName(data.QuestName)

	if len(planSummary) > 0 {
		builder = builder.SetPlanSummary(planSummary)
	}

	_, err = builder.Save(ctx)
	if err != nil {
		return fmt.Errorf("save session event: %w", err)
	}
	return nil
}

func (r *eventRepo) AppendAnswerEvent(ctx context.Context, data AnswerEventData) error {
	ctx = r.scope(ctx)
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	_, err = r.client.AnswerEvent.Create().
		SetSequence(seqNum).
		SetOwnerID(r.owner).
		SetSessionID(data.SessionID).
		SetSkillID(data.SkillID).
		SetTier(data.Tier).
		SetCategory(data.Category).
		SetQuestionText(data.QuestionText).
		SetCorrectAnswer(data.CorrectAnswer).
		SetLearnerAnswer(data.LearnerAnswer).
		SetCorrect(data.Correct).
		SetTimeMs(data.TimeMs).
		SetAnswerFormat(data.AnswerFormat).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("save answer event: %w", err)
	}
	return nil
}

func (r *eventRepo) AnswersForSession(ctx context.Context, sessionID string) ([]AnswerEventRecord, error) {
	ctx = r.scope(ctx)
	events, err := r.client.AnswerEvent.Query().
		Where(answerevent.OwnerID(r.owner), answerevent.SessionID(sessionID)).
		Order(ent.Asc(answerevent.FieldSequence)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query session answers: %w", err)
	}

	records := make([]AnswerEventRecord, len(events))
	for i, e := range events {
		records[i] = AnswerEventRecord{
			Sequence:      e.Sequence,
			Timestamp:     e.Timestamp,
			SessionID:     e.SessionID,
			SkillID:       e.SkillID,
			Tier:          e.Tier,
			Category:      e.Category,
			QuestionText:  e.QuestionText,
			CorrectAnswer: e.CorrectAnswer,
			LearnerAnswer: e.LearnerAnswer,
			Correct:       e.Correct,
			TimeMs:        e.TimeMs,
			AnswerFormat:  e.AnswerFormat,
		}
	}
	return records, nil
}

func (r *eventRepo) LatestAnswerTime(ctx context.Context, skillID string) (time.Time, error) {
	ctx = r.scope(ctx)
	ae, err := r.client.AnswerEvent.Query().
		Where(answerevent.OwnerID(r.owner), answerevent.SkillID(skillID)).
		Order(ent.Desc(answerevent.FieldTimestamp)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("query latest answer time: %w", err)
	}
	return ae.Timestamp, nil
}

func (r *eventRepo) SkillAccuracy(ctx context.Context, skillID string) (float64, error) {
	ctx = r.scope(ctx)
	events, err := r.client.AnswerEvent.Query().
		Where(answerevent.OwnerID(r.owner), answerevent.SkillID(skillID)).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query skill accuracy: %w", err)
	}
	if len(events) == 0 {
		return 0, nil
	}

	correct := 0
	for _, e := range events {
		if e.Correct {
			correct++
		}
	}
	return float64(correct) / float64(len(events)), nil
}

// AnswerTallies aggregates in SQL. Snapshot attempt counters are not a
// substitute: mastery resets them when a skill changes tier or decays.
func (r *eventRepo) AnswerTallies(ctx context.Context) ([]AnswerTally, error) {
	ctx = r.scope(ctx)
	var rows []struct {
		SkillID string `json:"skill_id"`
		Correct bool   `json:"correct"`
		Count   int    `json:"count"`
	}
	err := r.client.AnswerEvent.Query().
		Where(answerevent.OwnerID(r.owner)).
		GroupBy(answerevent.FieldSkillID, answerevent.FieldCorrect).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("query answer tallies: %w", err)
	}

	bySkill := make(map[string]*AnswerTally)
	for _, row := range rows {
		t, ok := bySkill[row.SkillID]
		if !ok {
			t = &AnswerTally{SkillID: row.SkillID}
			bySkill[row.SkillID] = t
		}
		t.Attempts += row.Count
		if row.Correct {
			t.Correct += row.Count
		}
	}
	out := make([]AnswerTally, 0, len(bySkill))
	for _, t := range bySkill {
		out = append(out, *t)
	}
	slices.SortFunc(out, func(a, b AnswerTally) int { return strings.Compare(a.SkillID, b.SkillID) })
	return out, nil
}

// CompletedSessionCount skips sessions that served no question: they are
// not practice, and the profile prompt reports this count as such.
func (r *eventRepo) CompletedSessionCount(ctx context.Context) (int, error) {
	ctx = r.scope(ctx)
	n, err := r.client.SessionEvent.Query().
		Where(sessionevent.OwnerID(r.owner), sessionevent.Action("end"), sessionevent.QuestionsServedGT(0)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count completed sessions: %w", err)
	}
	return n, nil
}
