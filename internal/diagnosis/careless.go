package diagnosis

// CarelessAccuracyThreshold is the minimum historical accuracy (exclusive)
// for a wrong answer to be classified as a careless error.
const CarelessAccuracyThreshold = 0.80

// CarelessClassifier flags wrong answers from high-accuracy learners as
// careless slips rather than knowledge gaps.
type CarelessClassifier struct{}

func (c *CarelessClassifier) Name() string { return "careless" }

func (c *CarelessClassifier) Classify(input *ClassifyInput) (ErrorCategory, float64) {
	if input.SkillAccuracy > CarelessAccuracyThreshold {
		return CategoryCareless, 0.8
	}
	return "", 0
}
