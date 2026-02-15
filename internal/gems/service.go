package gems

import (
	"context"
	"fmt"
	"time"

	"github.com/abhisek/mathiz/internal/store"
)

// Service manages gem computation and award tracking.
type Service struct {
	depthMap  *DepthMap
	eventRepo store.EventRepo

	// SessionGems accumulates gems awarded during the current session.
	SessionGems []GemAward
}

// NewService creates a GemService with precomputed depth map.
func NewService(eventRepo store.EventRepo) *Service {
	return &Service{
		depthMap:  ComputeDepthMap(),
		eventRepo: eventRepo,
	}
}

// AwardMastery awards a mastery gem for a newly mastered skill.
func (s *Service) AwardMastery(ctx context.Context, skillID, skillName, sessionID string) *GemAward {
	rarity := s.depthMap.RarityForSkill(skillID)
	award := &GemAward{
		Type:      GemMastery,
		Rarity:    rarity,
		SkillID:   skillID,
		SkillName: skillName,
		SessionID: sessionID,
		Reason:    fmt.Sprintf("Mastered %s", skillName),
		AwardedAt: time.Now(),
	}
	s.persist(ctx, award)
	s.SessionGems = append(s.SessionGems, *award)
	return award
}

// AwardRecovery awards a recovery gem for a recovered rusty skill.
func (s *Service) AwardRecovery(ctx context.Context, skillID, skillName, sessionID string) *GemAward {
	rarity := s.depthMap.RarityForSkill(skillID)
	award := &GemAward{
		Type:      GemRecovery,
		Rarity:    rarity,
		SkillID:   skillID,
		SkillName: skillName,
		SessionID: sessionID,
		Reason:    fmt.Sprintf("Recovered %s", skillName),
		AwardedAt: time.Now(),
	}
	s.persist(ctx, award)
	s.SessionGems = append(s.SessionGems, *award)
	return award
}

// AwardRetention awards a retention gem for a graduated skill.
func (s *Service) AwardRetention(ctx context.Context, skillID, skillName, sessionID string) *GemAward {
	rarity := s.depthMap.RarityForSkill(skillID)
	award := &GemAward{
		Type:      GemRetention,
		Rarity:    rarity,
		SkillID:   skillID,
		SkillName: skillName,
		SessionID: sessionID,
		Reason:    fmt.Sprintf("Retained %s", skillName),
		AwardedAt: time.Now(),
	}
	s.persist(ctx, award)
	s.SessionGems = append(s.SessionGems, *award)
	return award
}

// AwardStreak awards a streak gem for consecutive correct answers.
func (s *Service) AwardStreak(ctx context.Context, streakLength int, sessionID string) *GemAward {
	rarity := StreakRarity(streakLength)
	award := &GemAward{
		Type:      GemStreak,
		Rarity:    rarity,
		SessionID: sessionID,
		Reason:    fmt.Sprintf("%d correct in a row!", streakLength),
		AwardedAt: time.Now(),
	}
	s.persist(ctx, award)
	s.SessionGems = append(s.SessionGems, *award)
	return award
}

// AwardSession awards a session-completion gem.
func (s *Service) AwardSession(ctx context.Context, accuracy float64, sessionID string) *GemAward {
	rarity := SessionRarity(accuracy)
	award := &GemAward{
		Type:      GemSession,
		Rarity:    rarity,
		SessionID: sessionID,
		Reason:    fmt.Sprintf("Session complete (%.0f%% accuracy)", accuracy*100),
		AwardedAt: time.Now(),
	}
	s.persist(ctx, award)
	s.SessionGems = append(s.SessionGems, *award)
	return award
}

// ResetSession clears the session gem accumulator. Called at session start.
func (s *Service) ResetSession() {
	s.SessionGems = nil
}

// SnapshotData builds the gem counts for snapshot persistence.
func (s *Service) SnapshotData(ctx context.Context) *store.GemsSnapshotData {
	counts, total, _ := s.eventRepo.GemCounts(ctx)
	return &store.GemsSnapshotData{
		TotalCount:  total,
		CountByType: counts,
	}
}

func (s *Service) persist(ctx context.Context, award *GemAward) {
	if s.eventRepo == nil {
		return
	}
	data := store.GemEventData{
		GemType:   string(award.Type),
		Rarity:    string(award.Rarity),
		SessionID: award.SessionID,
		Reason:    award.Reason,
	}
	if award.SkillID != "" {
		data.SkillID = &award.SkillID
		data.SkillName = &award.SkillName
	}
	_ = s.eventRepo.AppendGemEvent(ctx, data)
}
