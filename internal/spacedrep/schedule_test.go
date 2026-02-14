package spacedrep

import "testing"

func TestBaseIntervals_Length(t *testing.T) {
	if len(BaseIntervals) != 6 {
		t.Errorf("expected 6 base intervals, got %d", len(BaseIntervals))
	}
}

func TestBaseIntervals_Values(t *testing.T) {
	expected := []int{1, 3, 7, 14, 30, 60}
	for i, v := range expected {
		if BaseIntervals[i] != v {
			t.Errorf("BaseIntervals[%d] = %d, want %d", i, BaseIntervals[i], v)
		}
	}
}

func TestConstants(t *testing.T) {
	if MaxStage != 5 {
		t.Errorf("MaxStage = %d, want 5", MaxStage)
	}
	if GraduationStage != 6 {
		t.Errorf("GraduationStage = %d, want 6", GraduationStage)
	}
	if GraduatedIntervalDays != 90 {
		t.Errorf("GraduatedIntervalDays = %d, want 90", GraduatedIntervalDays)
	}
}

func TestCurrentIntervalDays_EachStage(t *testing.T) {
	tests := []struct {
		stage    int
		expected int
	}{
		{0, 1},
		{1, 3},
		{2, 7},
		{3, 14},
		{4, 30},
		{5, 60},
	}
	for _, tt := range tests {
		rs := &ReviewState{Stage: tt.stage}
		got := rs.CurrentIntervalDays()
		if got != tt.expected {
			t.Errorf("Stage %d: CurrentIntervalDays() = %d, want %d", tt.stage, got, tt.expected)
		}
	}
}

func TestCurrentIntervalDays_BeyondMaxStage(t *testing.T) {
	rs := &ReviewState{Stage: 10}
	got := rs.CurrentIntervalDays()
	if got != 60 {
		t.Errorf("Stage 10: CurrentIntervalDays() = %d, want 60", got)
	}
}

func TestCurrentIntervalDays_Graduated(t *testing.T) {
	rs := &ReviewState{Stage: 6, Graduated: true}
	got := rs.CurrentIntervalDays()
	if got != 90 {
		t.Errorf("Graduated: CurrentIntervalDays() = %d, want 90", got)
	}
}
