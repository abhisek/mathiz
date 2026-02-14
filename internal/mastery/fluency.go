package mastery

import "github.com/abhisek/mathiz/internal/skillgraph"

const (
	// DefaultSpeedWindow is the default rolling window size for speed scores.
	DefaultSpeedWindow = 10

	// DefaultStreakCap is the default streak cap for consistency scoring.
	DefaultStreakCap = 8
)

// FluencyMetrics holds the raw data needed to compute a fluency score.
type FluencyMetrics struct {
	// SpeedScores holds the last N speed scores for rolling average.
	SpeedScores []float64 `json:"speed_scores"`
	// SpeedWindow is the rolling window size (default 10).
	SpeedWindow int `json:"speed_window"`
	// Streak is the current correct-answer streak.
	Streak int `json:"streak"`
	// StreakCap is the maximum streak value for consistency scoring (default 8).
	StreakCap int `json:"streak_cap"`
}

// DefaultFluencyMetrics returns a FluencyMetrics with default settings.
func DefaultFluencyMetrics() FluencyMetrics {
	return FluencyMetrics{
		SpeedWindow: DefaultSpeedWindow,
		StreakCap:    DefaultStreakCap,
	}
}

// FluencyScore computes the combined fluency score from metrics and accuracy.
func FluencyScore(metrics *FluencyMetrics, accuracy float64) float64 {
	speed := averageSpeed(metrics)
	consistency := ConsistencyScore(metrics.Streak, metrics.StreakCap)

	score := 0.6*clamp(accuracy, 0, 1) + 0.2*clamp(speed, 0, 1) + 0.2*clamp(consistency, 0, 1)
	return clamp(score, 0, 1)
}

// SpeedScore computes the speed component from response time and tier config.
func SpeedScore(responseTimeMs int, tierCfg skillgraph.TierConfig) float64 {
	if tierCfg.TimeLimitSecs == 0 {
		return 0.5 // Neutral for Learn tier
	}

	timeLimitMs := float64(tierCfg.TimeLimitSecs) * 1000
	ratio := float64(responseTimeMs) / timeLimitMs

	switch {
	case ratio <= 0.5:
		return 1.0
	case ratio <= 1.0:
		return 1.0 - (ratio - 0.5)
	default:
		return max(0.0, 0.5-0.5*(ratio-1.0))
	}
}

// ConsistencyScore computes the consistency component from the current streak.
func ConsistencyScore(streak int, cap int) float64 {
	if cap <= 0 {
		return 0.0
	}
	if streak >= cap {
		return 1.0
	}
	return float64(streak) / float64(cap)
}

// RecordSpeed adds a speed score to the rolling window.
func RecordSpeed(metrics *FluencyMetrics, score float64) {
	metrics.SpeedScores = append(metrics.SpeedScores, score)
	window := metrics.SpeedWindow
	if window <= 0 {
		window = DefaultSpeedWindow
	}
	if len(metrics.SpeedScores) > window {
		metrics.SpeedScores = metrics.SpeedScores[len(metrics.SpeedScores)-window:]
	}
}

func averageSpeed(metrics *FluencyMetrics) float64 {
	if len(metrics.SpeedScores) == 0 {
		return 0.5 // Neutral default
	}
	sum := 0.0
	for _, s := range metrics.SpeedScores {
		sum += s
	}
	return sum / float64(len(metrics.SpeedScores))
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
