package problemgen

import "testing"

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Validator: "test-validator",
		Message:   "something went wrong",
		Retryable: true,
	}
	expected := `validator "test-validator": something went wrong`
	if err.Error() != expected {
		t.Errorf("got %q, want %q", err.Error(), expected)
	}
}

func TestDefaultConfig_ValidatorChain(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.Validators) != 3 {
		t.Fatalf("expected 3 validators, got %d", len(cfg.Validators))
	}
	names := []string{"structural", "answer-format", "math-check"}
	for i, v := range cfg.Validators {
		if v.Name() != names[i] {
			t.Errorf("validator %d: expected %q, got %q", i, names[i], v.Name())
		}
	}
}

func TestDefaultConfig_Values(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxTokens != 512 {
		t.Errorf("expected MaxTokens 512, got %d", cfg.MaxTokens)
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("expected Temperature 0.7, got %f", cfg.Temperature)
	}
	if cfg.MaxPriorQuestions != 8 {
		t.Errorf("expected MaxPriorQuestions 8, got %d", cfg.MaxPriorQuestions)
	}
	if cfg.MaxRecentErrors != 5 {
		t.Errorf("expected MaxRecentErrors 5, got %d", cfg.MaxRecentErrors)
	}
}
