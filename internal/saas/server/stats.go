package server

import (
	"context"
	"time"

	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
)

// childSummary is the compact per-child card on the dashboard.
func (s *Server) childSummary(ctx context.Context, childUID string) (map[string]any, error) {
	snapRepo := s.st.SnapshotRepoFor(childUID)
	eventRepo := s.st.EventRepoFor(childUID)

	mastered, learning := 0, 0
	if snap, err := snapRepo.Latest(ctx); err != nil {
		return nil, err
	} else if snap != nil && snap.Data.Mastery != nil {
		for _, sk := range snap.Data.Mastery.Skills {
			switch sk.State {
			case "mastered":
				mastered++
			case "learning":
				learning++
			}
		}
	}

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
		t := sessions[0].Timestamp.UTC().Format(time.RFC3339)
		lastSessionAt = &t
	}

	return map[string]any{
		"masteredSkills": mastered,
		"learningSkills": learning,
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
	mastered, learning, rusty := 0, 0, 0

	snap, err := snapRepo.Latest(ctx)
	if err != nil {
		return nil, err
	}
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
			switch sk.State {
			case "mastered":
				mastered++
			case "learning":
				learning++
			case "rusty":
				rusty++
			}
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
			At:        sess.Timestamp.UTC().Format(time.RFC3339),
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
			"mastered": mastered,
			"learning": learning,
			"rusty":    rusty,
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
