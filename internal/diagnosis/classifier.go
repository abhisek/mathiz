package diagnosis

// Classifier is a rule-based error classifier.
// Returns a category and confidence (0.0–1.0), or ("", 0) if the rule doesn't apply.
type Classifier interface {
	Name() string
	Classify(input *ClassifyInput) (ErrorCategory, float64)
}

// DefaultClassifiers returns classifiers in priority order.
// Speed-rush has highest priority since a fast wrong answer is more likely
// a rush than a careless slip, even for high-accuracy learners.
func DefaultClassifiers() []Classifier {
	return []Classifier{
		&SpeedRushClassifier{},
		&CarelessClassifier{},
	}
}

// RunClassifiers executes rule-based classifiers in order.
// Returns the first match, or ("", 0, "") if no rules apply.
func RunClassifiers(classifiers []Classifier, input *ClassifyInput) (ErrorCategory, float64, string) {
	for _, c := range classifiers {
		cat, conf := c.Classify(input)
		if cat != "" {
			return cat, conf, c.Name()
		}
	}
	return "", 0, ""
}
