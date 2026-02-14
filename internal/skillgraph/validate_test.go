package skillgraph

import (
	"strings"
	"testing"
)

func TestValidate_SeedGraphPasses(t *testing.T) {
	err := Validate()
	if err != nil {
		t.Fatalf("seed graph validation failed: %v", err)
	}
}

func TestValidateSkills_DetectsCycle(t *testing.T) {
	skills := []Skill{
		{ID: "a", Strand: StrandAddSub, Prerequisites: []string{"b"}, Tiers: DefaultTiers()},
		{ID: "b", Strand: StrandAddSub, Prerequisites: []string{"a"}, Tiers: DefaultTiers()},
	}
	err := validateSkills(skills)
	if err == nil {
		t.Fatal("expected error for cycle, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention cycle, got: %v", err)
	}
}

func TestValidateSkills_DetectsDanglingPrereq(t *testing.T) {
	skills := []Skill{
		{ID: "a", Strand: StrandAddSub, Prerequisites: nil, Tiers: DefaultTiers()},
		{ID: "b", Strand: StrandAddSub, Prerequisites: []string{"nonexistent"}, Tiers: DefaultTiers()},
	}
	err := validateSkills(skills)
	if err == nil {
		t.Fatal("expected error for dangling prerequisite, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the missing ID, got: %v", err)
	}
}

func TestValidateSkills_DetectsDuplicateID(t *testing.T) {
	skills := []Skill{
		{ID: "a", Strand: StrandAddSub, Prerequisites: nil, Tiers: DefaultTiers()},
		{ID: "a", Strand: StrandAddSub, Prerequisites: nil, Tiers: DefaultTiers()},
	}
	err := validateSkills(skills)
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate, got: %v", err)
	}
}

func TestValidateSkills_RequiresAtLeastOneRoot(t *testing.T) {
	skills := []Skill{
		{ID: "a", Strand: StrandAddSub, Prerequisites: []string{"b"}, Tiers: DefaultTiers()},
		{ID: "b", Strand: StrandAddSub, Prerequisites: []string{"a"}, Tiers: DefaultTiers()},
	}
	err := validateSkills(skills)
	if err == nil {
		t.Fatal("expected error for no roots, got nil")
	}
	if !strings.Contains(err.Error(), "root") {
		t.Errorf("error should mention root, got: %v", err)
	}
}

func TestValidateSkills_AllStrandsPopulated(t *testing.T) {
	// Only one strand represented
	skills := []Skill{
		{ID: "a", Strand: StrandAddSub, Prerequisites: nil, Tiers: DefaultTiers()},
	}
	err := validateSkills(skills)
	if err == nil {
		t.Fatal("expected error for missing strands, got nil")
	}
	// Should mention at least one missing strand
	if !strings.Contains(err.Error(), "has no skills") {
		t.Errorf("error should mention missing strand, got: %v", err)
	}
}

func TestValidateSkills_InvalidTierConfig_ZeroProblems(t *testing.T) {
	skills := makeMinimalValidSkills()
	skills[0].Tiers[0].ProblemsRequired = 0
	err := validateSkills(skills)
	if err == nil {
		t.Fatal("expected error for zero ProblemsRequired, got nil")
	}
	if !strings.Contains(err.Error(), "ProblemsRequired") {
		t.Errorf("error should mention ProblemsRequired, got: %v", err)
	}
}

func TestValidateSkills_InvalidTierConfig_ZeroAccuracy(t *testing.T) {
	skills := makeMinimalValidSkills()
	skills[0].Tiers[0].AccuracyThreshold = 0
	err := validateSkills(skills)
	if err == nil {
		t.Fatal("expected error for zero AccuracyThreshold, got nil")
	}
	if !strings.Contains(err.Error(), "AccuracyThreshold") {
		t.Errorf("error should mention AccuracyThreshold, got: %v", err)
	}
}

func TestValidateSkills_InvalidTierConfig_ExcessiveAccuracy(t *testing.T) {
	skills := makeMinimalValidSkills()
	skills[0].Tiers[0].AccuracyThreshold = 1.5
	err := validateSkills(skills)
	if err == nil {
		t.Fatal("expected error for AccuracyThreshold > 1.0, got nil")
	}
}

func TestValidateSkills_InvalidTierConfig_NegativeTimeLimit(t *testing.T) {
	skills := makeMinimalValidSkills()
	skills[0].Tiers[1].TimeLimitSecs = -1
	err := validateSkills(skills)
	if err == nil {
		t.Fatal("expected error for negative TimeLimitSecs, got nil")
	}
	if !strings.Contains(err.Error(), "TimeLimitSecs") {
		t.Errorf("error should mention TimeLimitSecs, got: %v", err)
	}
}

// makeMinimalValidSkills returns a minimal set of skills covering all strands.
func makeMinimalValidSkills() []Skill {
	return []Skill{
		{ID: "s1", Strand: StrandNumberPlace, Tiers: DefaultTiers()},
		{ID: "s2", Strand: StrandAddSub, Tiers: DefaultTiers()},
		{ID: "s3", Strand: StrandMultDiv, Tiers: DefaultTiers()},
		{ID: "s4", Strand: StrandFractions, Tiers: DefaultTiers()},
		{ID: "s5", Strand: StrandMeasurement, Tiers: DefaultTiers()},
	}
}
