package session

import (
	"context"
	"sort"
	"time"

	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
)

// Planner builds a session plan from the current learner state.
type Planner interface {
	// BuildPlan creates a session plan.
	BuildPlan(mastered map[string]bool, tierProgress map[string]*TierProgress) (*Plan, error)
}

// DefaultPlanner implements the 60/30/10 planning strategy.
type DefaultPlanner struct {
	EventRepo store.EventRepo
	Ctx       context.Context
}

// NewPlanner creates a new DefaultPlanner.
func NewPlanner(ctx context.Context, eventRepo store.EventRepo) *DefaultPlanner {
	return &DefaultPlanner{
		EventRepo: eventRepo,
		Ctx:       ctx,
	}
}

// BuildPlan creates a session plan with the 60/30/10 mix.
func (p *DefaultPlanner) BuildPlan(mastered map[string]bool, tierProgress map[string]*TierProgress) (*Plan, error) {
	if mastered == nil {
		mastered = make(map[string]bool)
	}

	totalSlots := DefaultTotalSlots

	// Calculate slot allocation (60/30/10).
	frontierCount := 3
	reviewCount := 1
	boosterCount := 1

	// Get mastered skill IDs.
	var masteredIDs []string
	for id := range mastered {
		masteredIDs = append(masteredIDs, id)
	}
	sort.Strings(masteredIDs)

	hasMastered := len(masteredIDs) > 0

	// Redistribute if no mastered skills.
	if !hasMastered {
		frontierCount = totalSlots
		reviewCount = 0
		boosterCount = 0
	}

	// Select frontier skills.
	frontierSkills := selectFrontierSkills(mastered, frontierCount)

	// If no frontier skills available, redistribute to review/booster.
	if len(frontierSkills) == 0 && hasMastered {
		reviewCount += frontierCount
		frontierCount = 0
		// Cap review count at number of mastered skills.
		if reviewCount > len(masteredIDs) {
			boosterCount += reviewCount - len(masteredIDs)
			reviewCount = len(masteredIDs)
		}
	}

	var slots []PlanSlot

	// Add frontier slots.
	for i := 0; i < frontierCount && len(frontierSkills) > 0; i++ {
		skill := frontierSkills[i%len(frontierSkills)]
		tier := tierForSkill(skill.ID, tierProgress)
		slots = append(slots, PlanSlot{
			Skill:    skill,
			Tier:     tier,
			Category: CategoryFrontier,
		})
	}

	// Add review slot(s).
	if reviewCount > 0 && hasMastered {
		reviewSkills := p.selectReviewSkills(masteredIDs, reviewCount)
		for _, skill := range reviewSkills {
			tier := tierForSkill(skill.ID, tierProgress)
			slots = append(slots, PlanSlot{
				Skill:    skill,
				Tier:     tier,
				Category: CategoryReview,
			})
		}
		// Redistribute unused review slots to frontier.
		unused := reviewCount - len(reviewSkills)
		if unused > 0 && len(frontierSkills) > 0 {
			for i := 0; i < unused; i++ {
				skill := frontierSkills[i%len(frontierSkills)]
				tier := tierForSkill(skill.ID, tierProgress)
				slots = append(slots, PlanSlot{
					Skill:    skill,
					Tier:     tier,
					Category: CategoryFrontier,
				})
			}
		}
	}

	// Add booster slot(s).
	if boosterCount > 0 && hasMastered {
		boosterSkills := p.selectBoosterSkills(masteredIDs, boosterCount)
		for _, skill := range boosterSkills {
			slots = append(slots, PlanSlot{
				Skill:    skill,
				Tier:     skillgraph.TierLearn, // Booster always Learn tier
				Category: CategoryBooster,
			})
		}
		// Redistribute unused booster slots to frontier.
		unused := boosterCount - len(boosterSkills)
		if unused > 0 && len(frontierSkills) > 0 {
			for i := 0; i < unused; i++ {
				skill := frontierSkills[i%len(frontierSkills)]
				tier := tierForSkill(skill.ID, tierProgress)
				slots = append(slots, PlanSlot{
					Skill:    skill,
					Tier:     tier,
					Category: CategoryFrontier,
				})
			}
		}
	}

	// If we still have no slots (edge case: no skills at all), return empty plan.
	if len(slots) == 0 {
		return &Plan{Duration: DefaultSessionDuration}, nil
	}

	return &Plan{
		Slots:    slots,
		Duration: DefaultSessionDuration,
	}, nil
}

// selectFrontierSkills picks frontier skills prioritized by:
// 1. Lowest grade first
// 2. Most dependents within same grade
// 3. Alphabetical ID tiebreaker
func selectFrontierSkills(mastered map[string]bool, count int) []skillgraph.Skill {
	available := skillgraph.AvailableSkills(mastered)
	if len(available) == 0 {
		return nil
	}

	// Sort by priority.
	sort.Slice(available, func(i, j int) bool {
		if available[i].GradeLevel != available[j].GradeLevel {
			return available[i].GradeLevel < available[j].GradeLevel
		}
		depsI := len(skillgraph.Dependents(available[i].ID))
		depsJ := len(skillgraph.Dependents(available[j].ID))
		if depsI != depsJ {
			return depsI > depsJ
		}
		return available[i].ID < available[j].ID
	})

	if len(available) >= count {
		return available[:count]
	}
	return available
}

// selectReviewSkills picks mastered skills that were least recently practiced.
func (p *DefaultPlanner) selectReviewSkills(masteredIDs []string, count int) []skillgraph.Skill {
	type skillTime struct {
		skill skillgraph.Skill
		t     time.Time
	}

	var candidates []skillTime
	for _, id := range masteredIDs {
		skill, err := skillgraph.GetSkill(id)
		if err != nil {
			continue
		}
		t, err := p.EventRepo.LatestAnswerTime(p.Ctx, id)
		if err != nil {
			t = time.Time{} // Treat errors as "never practiced"
		}
		candidates = append(candidates, skillTime{skill: skill, t: t})
	}

	// Sort by oldest first (least recently practiced).
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].t.Before(candidates[j].t)
	})

	var result []skillgraph.Skill
	for i := 0; i < count && i < len(candidates); i++ {
		result = append(result, candidates[i].skill)
	}
	return result
}

// selectBoosterSkills picks mastered skills with highest historical accuracy.
func (p *DefaultPlanner) selectBoosterSkills(masteredIDs []string, count int) []skillgraph.Skill {
	type skillAcc struct {
		skill skillgraph.Skill
		acc   float64
	}

	var candidates []skillAcc
	for _, id := range masteredIDs {
		skill, err := skillgraph.GetSkill(id)
		if err != nil {
			continue
		}
		acc, err := p.EventRepo.SkillAccuracy(p.Ctx, id)
		if err != nil {
			acc = 0
		}
		candidates = append(candidates, skillAcc{skill: skill, acc: acc})
	}

	// Sort by highest accuracy first.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].acc != candidates[j].acc {
			return candidates[i].acc > candidates[j].acc
		}
		return candidates[i].skill.ID < candidates[j].skill.ID
	})

	var result []skillgraph.Skill
	for i := 0; i < count && i < len(candidates); i++ {
		result = append(result, candidates[i].skill)
	}
	return result
}

// tierForSkill returns the current tier for a skill based on tier progress.
func tierForSkill(skillID string, tierProgress map[string]*TierProgress) skillgraph.Tier {
	if tp, ok := tierProgress[skillID]; ok {
		return tp.CurrentTier
	}
	return skillgraph.TierLearn
}
