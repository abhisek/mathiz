package mastery

import (
	"math"
	"testing"

	"github.com/abhisek/mathiz/internal/skillgraph"
)

const epsilon = 0.001

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestFluencyScore_AllPerfect(t *testing.T) {
	metrics := &FluencyMetrics{
		SpeedScores: []float64{1.0, 1.0, 1.0},
		SpeedWindow: 10,
		Streak:      8,
		StreakCap:    8,
	}
	score := FluencyScore(metrics, 1.0)
	if !almostEqual(score, 1.0) {
		t.Errorf("FluencyScore = %f, want 1.0", score)
	}
}

func TestFluencyScore_ZeroAttempts(t *testing.T) {
	metrics := &FluencyMetrics{
		SpeedWindow: 10,
		StreakCap:    8,
	}
	// accuracy=0, speed=0.5 (neutral default), consistency=0
	// 0.0*0.6 + 0.5*0.2 + 0.0*0.2 = 0.1
	score := FluencyScore(metrics, 0.0)
	if !almostEqual(score, 0.1) {
		t.Errorf("FluencyScore = %f, want 0.1", score)
	}
}

func TestFluencyScore_MixedPerformance(t *testing.T) {
	metrics := &FluencyMetrics{
		SpeedScores: []float64{0.7},
		SpeedWindow: 10,
		Streak:      4,
		StreakCap:    8,
	}
	// 0.8*0.6 + 0.7*0.2 + 0.5*0.2 = 0.48 + 0.14 + 0.10 = 0.72
	score := FluencyScore(metrics, 0.8)
	if !almostEqual(score, 0.72) {
		t.Errorf("FluencyScore = %f, want 0.72", score)
	}
}

func TestFluencyScore_Clamping(t *testing.T) {
	metrics := &FluencyMetrics{
		SpeedScores: []float64{1.0, 1.0, 1.0},
		SpeedWindow: 10,
		Streak:      100,
		StreakCap:    8,
	}
	score := FluencyScore(metrics, 1.0)
	if score > 1.0 {
		t.Errorf("FluencyScore = %f, want <= 1.0", score)
	}
}

func TestSpeedScore_ProveTier_VeryFast(t *testing.T) {
	cfg := skillgraph.TierConfig{TimeLimitSecs: 30}
	// 10s / 30s = 0.33 ratio, < 0.5, so speed = 1.0
	score := SpeedScore(10000, cfg)
	if !almostEqual(score, 1.0) {
		t.Errorf("SpeedScore = %f, want 1.0", score)
	}
}

func TestSpeedScore_ProveTier_HalfTime(t *testing.T) {
	cfg := skillgraph.TierConfig{TimeLimitSecs: 30}
	// 15s / 30s = 0.5 ratio, at boundary, speed = 1.0
	score := SpeedScore(15000, cfg)
	if !almostEqual(score, 1.0) {
		t.Errorf("SpeedScore = %f, want 1.0", score)
	}
}

func TestSpeedScore_ProveTier_AtLimit(t *testing.T) {
	cfg := skillgraph.TierConfig{TimeLimitSecs: 30}
	// 30s / 30s = 1.0 ratio, speed = 1.0 - (1.0 - 0.5) = 0.5
	score := SpeedScore(30000, cfg)
	if !almostEqual(score, 0.5) {
		t.Errorf("SpeedScore = %f, want 0.5", score)
	}
}

func TestSpeedScore_ProveTier_Slow(t *testing.T) {
	cfg := skillgraph.TierConfig{TimeLimitSecs: 30}
	// 45s / 30s = 1.5 ratio, speed = max(0, 0.5 - 0.5*(1.5-1.0)) = max(0, 0.25) = 0.25
	score := SpeedScore(45000, cfg)
	if !almostEqual(score, 0.25) {
		t.Errorf("SpeedScore = %f, want 0.25", score)
	}
}

func TestSpeedScore_ProveTier_VeryOverTime(t *testing.T) {
	cfg := skillgraph.TierConfig{TimeLimitSecs: 30}
	// 60s / 30s = 2.0 ratio, speed = max(0, 0.5 - 0.5*1.0) = 0.0
	score := SpeedScore(60000, cfg)
	if !almostEqual(score, 0.0) {
		t.Errorf("SpeedScore = %f, want 0.0", score)
	}
}

func TestSpeedScore_LearnTier_Neutral(t *testing.T) {
	cfg := skillgraph.TierConfig{TimeLimitSecs: 0}
	score := SpeedScore(5000, cfg)
	if !almostEqual(score, 0.5) {
		t.Errorf("SpeedScore = %f, want 0.5", score)
	}
}

func TestSpeedScore_RollingAverage(t *testing.T) {
	metrics := &FluencyMetrics{
		SpeedWindow: 3,
		StreakCap:    8,
	}

	RecordSpeed(metrics, 1.0)
	RecordSpeed(metrics, 0.5)
	RecordSpeed(metrics, 0.8)

	avg := averageSpeed(metrics)
	expected := (1.0 + 0.5 + 0.8) / 3.0
	if !almostEqual(avg, expected) {
		t.Errorf("average = %f, want %f", avg, expected)
	}

	// Add a 4th score — oldest should be dropped.
	RecordSpeed(metrics, 0.6)
	if len(metrics.SpeedScores) != 3 {
		t.Errorf("SpeedScores length = %d, want 3", len(metrics.SpeedScores))
	}
	avg = averageSpeed(metrics)
	expected = (0.5 + 0.8 + 0.6) / 3.0
	if !almostEqual(avg, expected) {
		t.Errorf("average after slide = %f, want %f", avg, expected)
	}
}

func TestConsistencyScore_ZeroStreak(t *testing.T) {
	score := ConsistencyScore(0, 8)
	if score != 0.0 {
		t.Errorf("ConsistencyScore = %f, want 0.0", score)
	}
}

func TestConsistencyScore_PartialStreak(t *testing.T) {
	score := ConsistencyScore(4, 8)
	if !almostEqual(score, 0.5) {
		t.Errorf("ConsistencyScore = %f, want 0.5", score)
	}
}

func TestConsistencyScore_FullStreak(t *testing.T) {
	score := ConsistencyScore(8, 8)
	if score != 1.0 {
		t.Errorf("ConsistencyScore = %f, want 1.0", score)
	}
}

func TestConsistencyScore_OverCap(t *testing.T) {
	score := ConsistencyScore(12, 8)
	if score != 1.0 {
		t.Errorf("ConsistencyScore = %f, want 1.0 (clamped)", score)
	}
}

func TestConsistencyScore_ZeroCap(t *testing.T) {
	score := ConsistencyScore(5, 0)
	if score != 0.0 {
		t.Errorf("ConsistencyScore = %f, want 0.0 (zero cap)", score)
	}
}
