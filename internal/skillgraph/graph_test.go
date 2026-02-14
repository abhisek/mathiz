package skillgraph

import (
	"testing"
)

func TestGetSkill_Exists(t *testing.T) {
	s, err := GetSkill("pv-hundreds")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "Place Value to 1,000" {
		t.Errorf("got name %q, want %q", s.Name, "Place Value to 1,000")
	}
	if s.Strand != StrandNumberPlace {
		t.Errorf("got strand %q, want %q", s.Strand, StrandNumberPlace)
	}
	if s.GradeLevel != 3 {
		t.Errorf("got grade %d, want 3", s.GradeLevel)
	}
}

func TestGetSkill_NotFound(t *testing.T) {
	_, err := GetSkill("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent skill, got nil")
	}
}

func TestAllSkills_Count(t *testing.T) {
	all := AllSkills()
	if len(all) != 52 {
		t.Errorf("got %d skills, want 52", len(all))
	}
}

func TestByStrand(t *testing.T) {
	tests := []struct {
		strand Strand
		want   int
	}{
		{StrandNumberPlace, 8},
		{StrandAddSub, 10},
		{StrandMultDiv, 14},
		{StrandFractions, 12},
		{StrandMeasurement, 8},
	}
	for _, tt := range tests {
		skills := ByStrand(tt.strand)
		if len(skills) != tt.want {
			t.Errorf("ByStrand(%q): got %d skills, want %d", tt.strand, len(skills), tt.want)
		}
	}
}

func TestByStrand_SortedByGrade(t *testing.T) {
	for _, strand := range AllStrands() {
		skills := ByStrand(strand)
		for i := 1; i < len(skills); i++ {
			if skills[i].GradeLevel < skills[i-1].GradeLevel {
				t.Errorf("ByStrand(%q): skill %q (grade %d) appears after %q (grade %d)",
					strand, skills[i].ID, skills[i].GradeLevel, skills[i-1].ID, skills[i-1].GradeLevel)
			}
		}
	}
}

func TestByGrade(t *testing.T) {
	grade3 := ByGrade(3)
	grade4 := ByGrade(4)
	grade5 := ByGrade(5)
	grade6 := ByGrade(6)

	total := len(grade3) + len(grade4) + len(grade5)
	if total != 52 {
		t.Errorf("grade 3+4+5 total: got %d, want 52", total)
	}
	if len(grade6) != 0 {
		t.Errorf("ByGrade(6): got %d skills, want 0", len(grade6))
	}

	// Verify all skills in each result are the correct grade
	for _, s := range grade3 {
		if s.GradeLevel != 3 {
			t.Errorf("grade 3 result contains skill %q with grade %d", s.ID, s.GradeLevel)
		}
	}
	for _, s := range grade4 {
		if s.GradeLevel != 4 {
			t.Errorf("grade 4 result contains skill %q with grade %d", s.ID, s.GradeLevel)
		}
	}
}

func TestRootSkills(t *testing.T) {
	roots := RootSkills()
	if len(roots) == 0 {
		t.Fatal("expected at least one root skill")
	}
	for _, s := range roots {
		if len(s.Prerequisites) != 0 {
			t.Errorf("root skill %q has prerequisites: %v", s.ID, s.Prerequisites)
		}
	}
	// pv-hundreds should be a root
	found := false
	for _, s := range roots {
		if s.ID == "pv-hundreds" {
			found = true
			break
		}
	}
	if !found {
		t.Error("pv-hundreds should be a root skill")
	}
}

func TestPrerequisites(t *testing.T) {
	// add-3digit requires add-2digit
	prereqs := Prerequisites("add-3digit")
	if len(prereqs) != 1 {
		t.Fatalf("add-3digit: got %d prereqs, want 1", len(prereqs))
	}
	if prereqs[0].ID != "add-2digit" {
		t.Errorf("add-3digit prereq: got %q, want %q", prereqs[0].ID, "add-2digit")
	}

	// round-nearest-1000 requires two prerequisites
	prereqs = Prerequisites("round-nearest-1000")
	if len(prereqs) != 2 {
		t.Fatalf("round-nearest-1000: got %d prereqs, want 2", len(prereqs))
	}
	ids := map[string]bool{}
	for _, p := range prereqs {
		ids[p.ID] = true
	}
	if !ids["pv-ten-thousands"] || !ids["round-nearest-10-100"] {
		t.Errorf("round-nearest-1000 prereqs: got %v", ids)
	}

	// Root skill has no prerequisites
	prereqs = Prerequisites("pv-hundreds")
	if len(prereqs) != 0 {
		t.Errorf("pv-hundreds: got %d prereqs, want 0", len(prereqs))
	}
}

func TestDependents(t *testing.T) {
	deps := Dependents("pv-hundreds")
	if len(deps) == 0 {
		t.Fatal("pv-hundreds should have dependents")
	}
	depIDs := map[string]bool{}
	for _, d := range deps {
		depIDs[d.ID] = true
	}
	expected := []string{"compare-1000", "round-nearest-10-100", "pv-ten-thousands",
		"add-2digit", "sub-2digit", "meas-length", "meas-mass-volume"}
	for _, id := range expected {
		if !depIDs[id] {
			t.Errorf("pv-hundreds missing dependent %q", id)
		}
	}
}

func TestIsUnlocked(t *testing.T) {
	empty := map[string]bool{}

	// Root skill is always unlocked
	if !IsUnlocked("pv-hundreds", empty) {
		t.Error("pv-hundreds should be unlocked with empty mastered set")
	}

	// add-3digit requires add-2digit
	if IsUnlocked("add-3digit", empty) {
		t.Error("add-3digit should be locked with empty mastered set")
	}
	if !IsUnlocked("add-3digit", map[string]bool{"add-2digit": true}) {
		t.Error("add-3digit should be unlocked when add-2digit is mastered")
	}

	// round-nearest-1000 requires both pv-ten-thousands AND round-nearest-10-100
	partial := map[string]bool{"pv-ten-thousands": true}
	if IsUnlocked("round-nearest-1000", partial) {
		t.Error("round-nearest-1000 should be locked with only one of two prereqs")
	}
	both := map[string]bool{"pv-ten-thousands": true, "round-nearest-10-100": true}
	if !IsUnlocked("round-nearest-1000", both) {
		t.Error("round-nearest-1000 should be unlocked with both prereqs mastered")
	}
}

func TestAvailableSkills_EmptyMastered(t *testing.T) {
	empty := map[string]bool{}
	available := AvailableSkills(empty)

	// With empty mastered, only root skills should be available
	roots := RootSkills()
	if len(available) != len(roots) {
		t.Errorf("got %d available skills with empty mastered, want %d (root count)", len(available), len(roots))
	}
	for _, s := range available {
		if len(s.Prerequisites) != 0 {
			t.Errorf("non-root skill %q is available with empty mastered set", s.ID)
		}
	}
}

func TestAvailableSkills_PartialMastered(t *testing.T) {
	mastered := map[string]bool{"pv-hundreds": true}
	available := AvailableSkills(mastered)

	// pv-hundreds is mastered, so it should NOT be in available
	for _, s := range available {
		if s.ID == "pv-hundreds" {
			t.Error("mastered skill pv-hundreds should not be in available set")
		}
	}

	// Skills that depend only on pv-hundreds should now be available
	expectedAvailable := map[string]bool{
		"compare-1000":        true,
		"round-nearest-10-100": true,
		"pv-ten-thousands":    true,
		"add-2digit":          true,
		"sub-2digit":          true,
		"meas-length":         true,
		"meas-mass-volume":    true,
	}
	availableIDs := map[string]bool{}
	for _, s := range available {
		availableIDs[s.ID] = true
	}
	for id := range expectedAvailable {
		if !availableIDs[id] {
			t.Errorf("expected %q to be available, but it wasn't", id)
		}
	}
}

func TestFrontierSkills(t *testing.T) {
	empty := map[string]bool{}
	frontier := FrontierSkills(empty)

	// With empty mastered, frontier should return root skills
	if len(frontier) == 0 {
		t.Fatal("frontier should not be empty with empty mastered set")
	}

	// Frontier must be a subset of available
	available := AvailableSkills(empty)
	availableIDs := map[string]bool{}
	for _, s := range available {
		availableIDs[s.ID] = true
	}
	for _, s := range frontier {
		if !availableIDs[s.ID] {
			t.Errorf("frontier skill %q is not in available set", s.ID)
		}
	}

	// With some mastered, frontier should prefer non-root skills
	mastered := map[string]bool{"pv-hundreds": true}
	frontier = FrontierSkills(mastered)
	for _, s := range frontier {
		if len(s.Prerequisites) == 0 {
			t.Errorf("frontier should prefer non-root skills when available, got root %q", s.ID)
		}
	}
}

func TestBlockedSkills_EmptyMastered(t *testing.T) {
	empty := map[string]bool{}
	blocked := BlockedSkills(empty)
	roots := RootSkills()

	// Everything except roots should be blocked
	expectedBlocked := 52 - len(roots)
	if len(blocked) != expectedBlocked {
		t.Errorf("got %d blocked skills, want %d", len(blocked), expectedBlocked)
	}
}

func TestBlockedSkills_AllMastered(t *testing.T) {
	all := AllSkills()
	mastered := make(map[string]bool, len(all))
	for _, s := range all {
		mastered[s.ID] = true
	}

	blocked := BlockedSkills(mastered)
	if len(blocked) != 0 {
		t.Errorf("got %d blocked skills with all mastered, want 0", len(blocked))
	}
}

func TestTopologicalOrder(t *testing.T) {
	topo := TopologicalOrder()
	if len(topo) != 52 {
		t.Fatalf("got %d skills in topo order, want 52", len(topo))
	}

	// Verify topological property: every skill appears after all its prerequisites
	posMap := make(map[string]int, len(topo))
	for i, s := range topo {
		posMap[s.ID] = i
	}

	for _, s := range topo {
		for _, prereqID := range s.Prerequisites {
			prereqPos, ok := posMap[prereqID]
			if !ok {
				t.Errorf("prerequisite %q of %q not found in topo order", prereqID, s.ID)
				continue
			}
			skillPos := posMap[s.ID]
			if prereqPos >= skillPos {
				t.Errorf("skill %q (pos %d) appears before prerequisite %q (pos %d)",
					s.ID, skillPos, prereqID, prereqPos)
			}
		}
	}
}

func TestAllSkills_ReturnsCopy(t *testing.T) {
	a := AllSkills()
	b := AllSkills()
	if len(a) != len(b) {
		t.Fatal("AllSkills returned different lengths")
	}
	// Mutating one should not affect the other
	a[0].Name = "MUTATED"
	c := AllSkills()
	if c[0].Name == "MUTATED" {
		t.Error("AllSkills did not return a defensive copy")
	}
}
