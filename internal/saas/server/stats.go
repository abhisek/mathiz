package server

import (
	"context"

	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
)

// masteryTally counts skills by mastery state in a snapshot. Both the
// dashboard card and the detail view derive their numbers from this one
// function so they can never disagree.
type masteryTally struct {
	mastered, learning, rusty int
}

func tallyMastery(snap *store.Snapshot) masteryTally {
	var t masteryTally
	if snap == nil || snap.Data.Mastery == nil {
		return t
	}
	for _, sk := range snap.Data.Mastery.Skills {
		switch sk.State {
		case "mastered":
			t.mastered++
		case "learning":
			t.learning++
		case "rusty":
			t.rusty++
		}
	}
	return t
}

// childSummary is the compact per-child card on the dashboard.
func (s *Server) childSummary(ctx context.Context, childUID string) (map[string]any, error) {
	snapRepo := s.st.SnapshotRepoFor(childUID)
	eventRepo := s.st.EventRepoFor(childUID)

	snap, err := snapRepo.Latest(ctx)
	if err != nil {
		return nil, err
	}
	tally := tallyMastery(snap)

	_, gemTotal, err := eventRepo.GemCounts(ctx)
	if err != nil {
		return nil, err
	}

	sessions, err := eventRepo.QuerySessionSummaries(ctx, store.QueryOpts{Limit: 1})
	if err != nil {
		return nil, err
	}
	var lastSessionAt *string
	if len(sessions) > 0 {
		lastSessionAt = rfc3339Ptr(&sessions[0].Timestamp)
	}

	return map[string]any{
		"masteredSkills": tally.mastered,
		"learningSkills": tally.learning,
		"totalSkills":    len(skillgraph.AllSkills()),
		"gems":           gemTotal,
		"lastSessionAt":  lastSessionAt,
	}, nil
}

// childStats is the full per-child progress view.
func (s *Server) childStats(ctx context.Context, childUID string) (map[string]any, error) {
	snapRepo := s.st.SnapshotRepoFor(childUID)
	eventRepo := s.st.EventRepoFor(childUID)

	// Per-skill mastery from the latest snapshot.
	type skillStat struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Strand   string  `json:"strand"`
		Grade    int     `json:"grade"`
		State    string  `json:"state"`
		Accuracy float64 `json:"accuracy"`
		Attempts int     `json:"attempts"`
	}
	var skills []skillStat

	snap, err := snapRepo.Latest(ctx)
	if err != nil {
		return nil, err
	}
	tally := tallyMastery(snap)
	if snap != nil && snap.Data.Mastery != nil {
		for id, sk := range snap.Data.Mastery.Skills {
			meta, err := skillgraph.GetSkill(id)
			if err != nil {
				continue // skill removed from the graph; skip stale entries
			}
			acc := 0.0
			if sk.TotalAttempts > 0 {
				acc = float64(sk.CorrectCount) / float64(sk.TotalAttempts)
			}
			skills = append(skills, skillStat{
				ID: id, Name: meta.Name, Strand: skillgraph.StrandDisplayName(meta.Strand),
				Grade: meta.GradeLevel, State: sk.State,
				Accuracy: acc, Attempts: sk.TotalAttempts,
			})
		}
	}

	// Learner profile — the AI's evolving picture of this child.
	var profile map[string]any
	if snap != nil && snap.Data.LearnerProfile != nil {
		lp := snap.Data.LearnerProfile
		profile = map[string]any{
			"summary":    lp.Summary,
			"strengths":  lp.Strengths,
			"weaknesses": lp.Weaknesses,
			"patterns":   lp.Patterns,
		}
	}

	// Recent sessions.
	sessions, err := eventRepo.QuerySessionSummaries(ctx, store.QueryOpts{Limit: 10})
	if err != nil {
		return nil, err
	}
	type sessionStat struct {
		At           string `json:"at"`
		Questions    int    `json:"questions"`
		Correct      int    `json:"correct"`
		DurationSecs int    `json:"durationSecs"`
		Gems         int    `json:"gems"`
	}
	recent := make([]sessionStat, len(sessions))
	for i, sess := range sessions {
		recent[i] = sessionStat{
			At:        rfc3339(sess.Timestamp),
			Questions: sess.QuestionsServed, Correct: sess.CorrectAnswers,
			DurationSecs: sess.DurationSecs, Gems: sess.GemCount,
		}
	}

	gemsByType, gemTotal, err := eventRepo.GemCounts(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"mastery": map[string]any{
			"mastered": tally.mastered,
			"learning": tally.learning,
			"rusty":    tally.rusty,
			"total":    len(skillgraph.AllSkills()),
			"skills":   skills,
		},
		"learnerProfile": profile,
		"recentSessions": recent,
		"gems": map[string]any{
			"total":  gemTotal,
			"byType": gemsByType,
		},
	}, nil
}
