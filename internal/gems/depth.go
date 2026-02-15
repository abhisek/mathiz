package gems

import (
	"sort"

	"github.com/abhisek/mathiz/internal/skillgraph"
)

// DepthMap holds the DAG depth for each skill and the quartile boundaries.
type DepthMap struct {
	Depths     map[string]int // skillID → depth (longest path from root)
	Boundaries [3]int         // Q1/Q2, Q2/Q3, Q3/Q4 boundaries
}

// ComputeDepthMap computes DAG depths for all skills and quartile boundaries.
// Depth = longest path from any root to this skill.
func ComputeDepthMap() *DepthMap {
	skills := skillgraph.TopologicalOrder()
	depths := make(map[string]int, len(skills))

	// In topological order, compute longest path from root.
	for _, s := range skills {
		depth := 0
		for _, prereqID := range s.Prerequisites {
			if d, ok := depths[prereqID]; ok && d+1 > depth {
				depth = d + 1
			}
		}
		depths[s.ID] = depth
	}

	// Collect all depths and sort for quartile computation.
	vals := make([]int, 0, len(depths))
	for _, d := range depths {
		vals = append(vals, d)
	}
	sort.Ints(vals)

	n := len(vals)
	var boundaries [3]int
	if n > 0 {
		boundaries = [3]int{
			vals[n/4],   // Q1/Q2 boundary
			vals[n/2],   // Q2/Q3 boundary
			vals[3*n/4], // Q3/Q4 boundary
		}
	}

	return &DepthMap{Depths: depths, Boundaries: boundaries}
}

// RarityForSkill returns the rarity based on a skill's DAG depth.
func (dm *DepthMap) RarityForSkill(skillID string) Rarity {
	depth := dm.Depths[skillID]
	switch {
	case depth > dm.Boundaries[2]:
		return RarityLegendary
	case depth > dm.Boundaries[1]:
		return RarityEpic
	case depth > dm.Boundaries[0]:
		return RarityRare
	default:
		return RarityCommon
	}
}
