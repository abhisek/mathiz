package diagnosis

import (
	"testing"

	"github.com/abhisek/mathiz/internal/skillgraph"
)

func TestAllMisconceptions_Count(t *testing.T) {
	all := AllMisconceptions()
	if len(all) != 19 {
		t.Errorf("got %d misconceptions, want 19", len(all))
	}
}

func TestGetMisconception_Found(t *testing.T) {
	m := GetMisconception("add-no-carry")
	if m == nil {
		t.Fatal("GetMisconception(add-no-carry) returned nil")
	}
	if m.Strand != skillgraph.StrandAddSub {
		t.Errorf("strand = %q, want %q", m.Strand, skillgraph.StrandAddSub)
	}
	if m.Label == "" {
		t.Error("label is empty")
	}
	if m.Description == "" {
		t.Error("description is empty")
	}
}

func TestGetMisconception_NotFound(t *testing.T) {
	m := GetMisconception("nonexistent")
	if m != nil {
		t.Errorf("GetMisconception(nonexistent) = %v, want nil", m)
	}
}

func TestMisconceptionsByStrand(t *testing.T) {
	tests := []struct {
		strand skillgraph.Strand
		want   int
	}{
		{skillgraph.StrandNumberPlace, 4},
		{skillgraph.StrandAddSub, 4},
		{skillgraph.StrandMultDiv, 4},
		{skillgraph.StrandFractions, 4},
		{skillgraph.StrandMeasurement, 3},
	}

	for _, tt := range tests {
		ms := MisconceptionsByStrand(tt.strand)
		if len(ms) != tt.want {
			t.Errorf("MisconceptionsByStrand(%s) = %d entries, want %d", tt.strand, len(ms), tt.want)
		}
	}
}

func TestMisconceptionsByStrand_Unknown(t *testing.T) {
	ms := MisconceptionsByStrand("nonexistent")
	if len(ms) != 0 {
		t.Errorf("MisconceptionsByStrand(nonexistent) = %d, want 0", len(ms))
	}
}

func TestSeedData_UniqueIDs(t *testing.T) {
	seen := make(map[string]bool)
	for _, m := range seedMisconceptions {
		if seen[m.ID] {
			t.Errorf("duplicate misconception ID: %s", m.ID)
		}
		seen[m.ID] = true
	}
}

func TestSeedData_AllFieldsPopulated(t *testing.T) {
	for _, m := range seedMisconceptions {
		if m.ID == "" {
			t.Error("misconception with empty ID")
		}
		if m.Strand == "" {
			t.Errorf("misconception %s has empty strand", m.ID)
		}
		if m.Label == "" {
			t.Errorf("misconception %s has empty label", m.ID)
		}
		if m.Description == "" {
			t.Errorf("misconception %s has empty description", m.ID)
		}
	}
}
