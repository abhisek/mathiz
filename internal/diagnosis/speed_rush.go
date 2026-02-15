package diagnosis

// SpeedRushThresholdMs is the maximum response time (exclusive) for a
// wrong answer to be classified as a speed-rush.
const SpeedRushThresholdMs = 2000

// SpeedRushClassifier flags answers submitted too quickly as speed-rush errors.
type SpeedRushClassifier struct{}

func (c *SpeedRushClassifier) Name() string { return "speed-rush" }

func (c *SpeedRushClassifier) Classify(input *ClassifyInput) (ErrorCategory, float64) {
	if input.ResponseTimeMs < SpeedRushThresholdMs {
		return CategorySpeedRush, 0.9
	}
	return "", 0
}
