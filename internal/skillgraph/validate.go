package skillgraph

import (
	"fmt"
	"strings"
)

// validateSkills performs all structural checks on the given skill set.
// Returns a combined error describing all problems found, or nil if valid.
func validateSkills(skills []Skill) error {
	var errs []string

	idSet := make(map[string]bool, len(skills))
	strandSet := make(map[Strand]bool)

	// Check for duplicate IDs
	for _, s := range skills {
		if idSet[s.ID] {
			errs = append(errs, fmt.Sprintf("duplicate skill ID: %q", s.ID))
		}
		idSet[s.ID] = true
		strandSet[s.Strand] = true
	}

	// Check for dangling prerequisites
	for _, s := range skills {
		for _, prereqID := range s.Prerequisites {
			if !idSet[prereqID] {
				errs = append(errs, fmt.Sprintf("skill %q references nonexistent prerequisite %q", s.ID, prereqID))
			}
		}
	}

	// Check for cycles using Kahn's algorithm
	inDegree := make(map[string]int, len(skills))
	adjList := make(map[string][]string)
	for _, s := range skills {
		inDegree[s.ID] = len(s.Prerequisites)
		for _, prereqID := range s.Prerequisites {
			adjList[prereqID] = append(adjList[prereqID], s.ID)
		}
	}

	var queue []string
	for _, s := range skills {
		if inDegree[s.ID] == 0 {
			queue = append(queue, s.ID)
		}
	}

	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, depID := range adjList[id] {
			inDegree[depID]--
			if inDegree[depID] == 0 {
				queue = append(queue, depID)
			}
		}
	}

	if visited < len(skills) {
		var cycleNodes []string
		for _, s := range skills {
			if inDegree[s.ID] > 0 {
				cycleNodes = append(cycleNodes, s.ID)
			}
		}
		errs = append(errs, fmt.Sprintf("cycle detected involving skills: %s", strings.Join(cycleNodes, ", ")))
	}

	// Check at least one root
	hasRoot := false
	for _, s := range skills {
		if len(s.Prerequisites) == 0 {
			hasRoot = true
			break
		}
	}
	if !hasRoot {
		errs = append(errs, "no root skills found (at least one skill must have no prerequisites)")
	}

	// Check all declared strands are populated
	for _, strand := range AllStrands() {
		if !strandSet[strand] {
			errs = append(errs, fmt.Sprintf("strand %q has no skills", strand))
		}
	}

	// Check tier configs are valid
	for _, s := range skills {
		for i, tc := range s.Tiers {
			prefix := fmt.Sprintf("skill %q tier %d", s.ID, i)
			if tc.ProblemsRequired <= 0 {
				errs = append(errs, fmt.Sprintf("%s: ProblemsRequired must be > 0, got %d", prefix, tc.ProblemsRequired))
			}
			if tc.AccuracyThreshold <= 0 || tc.AccuracyThreshold > 1.0 {
				errs = append(errs, fmt.Sprintf("%s: AccuracyThreshold must be in (0, 1.0], got %f", prefix, tc.AccuracyThreshold))
			}
			if tc.TimeLimitSecs < 0 {
				errs = append(errs, fmt.Sprintf("%s: TimeLimitSecs must be >= 0, got %d", prefix, tc.TimeLimitSecs))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("skill graph validation failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}
