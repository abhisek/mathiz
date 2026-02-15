package gems

import "testing"

func TestNextStreakThreshold(t *testing.T) {
	tests := []struct {
		current int
		want    int
	}{
		{0, 5},
		{1, 5},
		{4, 5},
		{5, 10},
		{9, 10},
		{10, 15},
		{14, 15},
		{15, 20},
		{19, 20},
		{20, 25},
		{24, 25},
		{25, 30},
	}

	for _, tt := range tests {
		got := NextStreakThreshold(tt.current)
		if got != tt.want {
			t.Errorf("NextStreakThreshold(%d) = %d, want %d", tt.current, got, tt.want)
		}
	}
}

func TestBaseStreakThreshold(t *testing.T) {
	if BaseStreakThreshold != 5 {
		t.Errorf("BaseStreakThreshold = %d, want 5", BaseStreakThreshold)
	}
}
