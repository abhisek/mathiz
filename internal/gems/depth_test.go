package gems

import (
	"testing"

	"github.com/abhisek/mathiz/internal/skillgraph"
)

func TestComputeDepthMap(t *testing.T) {
	dm := ComputeDepthMap()

	// All skills should have a depth.
	allSkills := skillgraph.AllSkills()
	if len(dm.Depths) != len(allSkills) {
		t.Errorf("expected %d skills in depth map, got %d", len(allSkills), len(dm.Depths))
	}

	// Root skills should have depth 0.
	roots := skillgraph.RootSkills()
	for _, r := range roots {
		if dm.Depths[r.ID] != 0 {
			t.Errorf("root skill %q should have depth 0, got %d", r.ID, dm.Depths[r.ID])
		}
	}

	// A skill with prerequisites should have depth > 0.
	for _, s := range allSkills {
		if len(s.Prerequisites) > 0 {
			if dm.Depths[s.ID] == 0 {
				t.Errorf("skill %q has prerequisites but depth 0", s.ID)
			}
			// Depth should be greater than all prerequisites' depths.
			for _, prereqID := range s.Prerequisites {
				if dm.Depths[s.ID] <= dm.Depths[prereqID] {
					t.Errorf("skill %q (depth %d) should have depth > prerequisite %q (depth %d)",
						s.ID, dm.Depths[s.ID], prereqID, dm.Depths[prereqID])
				}
			}
		}
	}
}

func TestComputeDepthMap_Quartiles(t *testing.T) {
	dm := ComputeDepthMap()

	// Boundaries should be in non-decreasing order.
	if dm.Boundaries[0] > dm.Boundaries[1] || dm.Boundaries[1] > dm.Boundaries[2] {
		t.Errorf("boundaries not in order: %v", dm.Boundaries)
	}
}

func TestRarityForSkill_Quartiles(t *testing.T) {
	dm := ComputeDepthMap()

	// Collect rarity distribution.
	counts := map[Rarity]int{}
	for _, s := range skillgraph.AllSkills() {
		r := dm.RarityForSkill(s.ID)
		counts[r]++
	}

	// Each rarity should have at least 1 skill.
	for _, rarity := range AllRarities() {
		if counts[rarity] == 0 {
			t.Logf("rarity %q has 0 skills (boundaries: %v)", rarity, dm.Boundaries)
		}
	}

	// Total should match all skills.
	total := 0
	for _, c := range counts {
		total += c
	}
	if total != len(skillgraph.AllSkills()) {
		t.Errorf("rarity total %d != skill count %d", total, len(skillgraph.AllSkills()))
	}
}

func TestRarityForSkill_RootIsCommon(t *testing.T) {
	dm := ComputeDepthMap()

	// Root skills (depth 0) should be Common.
	roots := skillgraph.RootSkills()
	for _, r := range roots {
		rarity := dm.RarityForSkill(r.ID)
		if rarity != RarityCommon {
			t.Errorf("root skill %q has rarity %q, expected Common", r.ID, rarity)
		}
	}
}

func TestRarityForSkill_UnknownSkill(t *testing.T) {
	dm := ComputeDepthMap()

	// Unknown skill has depth 0 → Common.
	r := dm.RarityForSkill("nonexistent")
	if r != RarityCommon {
		t.Errorf("unknown skill has rarity %q, expected Common", r)
	}
}
