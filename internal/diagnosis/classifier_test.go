package diagnosis

import (
	"testing"

	"github.com/abhisek/mathiz/internal/problemgen"
)

func TestSpeedRushClassifier_UnderThreshold(t *testing.T) {
	c := &SpeedRushClassifier{}
	cat, conf := c.Classify(&ClassifyInput{ResponseTimeMs: 1500})
	if cat != CategorySpeedRush {
		t.Errorf("got category %q, want %q", cat, CategorySpeedRush)
	}
	if conf != 0.9 {
		t.Errorf("got confidence %f, want 0.9", conf)
	}
}

func TestSpeedRushClassifier_AtThreshold(t *testing.T) {
	c := &SpeedRushClassifier{}
	cat, _ := c.Classify(&ClassifyInput{ResponseTimeMs: 2000})
	if cat != "" {
		t.Errorf("got category %q at threshold, want empty", cat)
	}
}

func TestSpeedRushClassifier_OverThreshold(t *testing.T) {
	c := &SpeedRushClassifier{}
	cat, _ := c.Classify(&ClassifyInput{ResponseTimeMs: 3000})
	if cat != "" {
		t.Errorf("got category %q, want empty", cat)
	}
}

func TestCarelessClassifier_HighAccuracy(t *testing.T) {
	c := &CarelessClassifier{}
	cat, conf := c.Classify(&ClassifyInput{SkillAccuracy: 0.85})
	if cat != CategoryCareless {
		t.Errorf("got category %q, want %q", cat, CategoryCareless)
	}
	if conf != 0.8 {
		t.Errorf("got confidence %f, want 0.8", conf)
	}
}

func TestCarelessClassifier_AtThreshold(t *testing.T) {
	c := &CarelessClassifier{}
	cat, _ := c.Classify(&ClassifyInput{SkillAccuracy: 0.80})
	if cat != "" {
		t.Errorf("got category %q at threshold, want empty", cat)
	}
}

func TestCarelessClassifier_LowAccuracy(t *testing.T) {
	c := &CarelessClassifier{}
	cat, _ := c.Classify(&ClassifyInput{SkillAccuracy: 0.60})
	if cat != "" {
		t.Errorf("got category %q, want empty", cat)
	}
}

func TestRunClassifiers_SpeedRushPriority(t *testing.T) {
	// Both speed-rush AND careless match → speed-rush wins.
	input := &ClassifyInput{
		ResponseTimeMs: 1000,
		SkillAccuracy:  0.90,
	}
	cat, _, name := RunClassifiers(DefaultClassifiers(), input)
	if cat != CategorySpeedRush {
		t.Errorf("got category %q, want %q (speed-rush should take priority)", cat, CategorySpeedRush)
	}
	if name != "speed-rush" {
		t.Errorf("got classifier %q, want %q", name, "speed-rush")
	}
}

func TestRunClassifiers_CarelessFallback(t *testing.T) {
	// Slow answer, high accuracy → careless.
	input := &ClassifyInput{
		ResponseTimeMs: 5000,
		SkillAccuracy:  0.90,
	}
	cat, _, name := RunClassifiers(DefaultClassifiers(), input)
	if cat != CategoryCareless {
		t.Errorf("got category %q, want %q", cat, CategoryCareless)
	}
	if name != "careless" {
		t.Errorf("got classifier %q, want %q", name, "careless")
	}
}

func TestRunClassifiers_NoMatch(t *testing.T) {
	// Slow answer, low accuracy → no rule matches.
	input := &ClassifyInput{
		ResponseTimeMs: 5000,
		SkillAccuracy:  0.50,
	}
	cat, conf, name := RunClassifiers(DefaultClassifiers(), input)
	if cat != "" {
		t.Errorf("got category %q, want empty", cat)
	}
	if conf != 0 {
		t.Errorf("got confidence %f, want 0", conf)
	}
	if name != "" {
		t.Errorf("got classifier %q, want empty", name)
	}
}

func TestRunClassifiers_NilQuestion(t *testing.T) {
	// ClassifyInput with nil question should still work (classifiers don't use it).
	input := &ClassifyInput{
		Question:       nil,
		ResponseTimeMs: 1000,
		SkillAccuracy:  0.50,
	}
	cat, _, _ := RunClassifiers(DefaultClassifiers(), input)
	if cat != CategorySpeedRush {
		t.Errorf("got %q, want %q", cat, CategorySpeedRush)
	}
}

func TestDefaultClassifiers_Order(t *testing.T) {
	classifiers := DefaultClassifiers()
	if len(classifiers) != 2 {
		t.Fatalf("got %d classifiers, want 2", len(classifiers))
	}
	if classifiers[0].Name() != "speed-rush" {
		t.Errorf("first classifier is %q, want speed-rush", classifiers[0].Name())
	}
	if classifiers[1].Name() != "careless" {
		t.Errorf("second classifier is %q, want careless", classifiers[1].Name())
	}
}

func TestSpeedRushClassifier_ZeroTime(t *testing.T) {
	c := &SpeedRushClassifier{}
	cat, _ := c.Classify(&ClassifyInput{
		Question:       &problemgen.Question{Text: "1+1?", Answer: "2"},
		ResponseTimeMs: 0,
	})
	if cat != CategorySpeedRush {
		t.Errorf("got %q, want %q for 0ms response", cat, CategorySpeedRush)
	}
}
