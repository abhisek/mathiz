package diagnosis

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/skillgraph"
)

func testQuestion() *problemgen.Question {
	skills := skillgraph.AllSkills()
	// Find an addition skill to match the taxonomy.
	var skillID string
	for _, s := range skills {
		if s.Strand == skillgraph.StrandAddSub {
			skillID = s.ID
			break
		}
	}
	if skillID == "" {
		skillID = skills[0].ID
	}
	return &problemgen.Question{
		Text:       "What is 47 + 38?",
		Answer:     "85",
		AnswerType: problemgen.AnswerTypeInteger,
		SkillID:    skillID,
		Tier:       skillgraph.TierLearn,
	}
}

func TestService_RuleBasedSpeedRush(t *testing.T) {
	svc := NewService(nil) // No LLM

	result := svc.Diagnose(context.Background(), testQuestion(), "715", 1500, 0.50, nil)
	if result.Category != CategorySpeedRush {
		t.Errorf("got %q, want %q", result.Category, CategorySpeedRush)
	}
	if result.ClassifierName != "speed-rush" {
		t.Errorf("got classifier %q, want speed-rush", result.ClassifierName)
	}
	svc.Close()
}

func TestService_RuleBasedCareless(t *testing.T) {
	svc := NewService(nil)

	result := svc.Diagnose(context.Background(), testQuestion(), "715", 5000, 0.90, nil)
	if result.Category != CategoryCareless {
		t.Errorf("got %q, want %q", result.Category, CategoryCareless)
	}
	svc.Close()
}

func TestService_UnclassifiedWithoutLLM(t *testing.T) {
	svc := NewService(nil)

	result := svc.Diagnose(context.Background(), testQuestion(), "715", 5000, 0.40, nil)
	if result.Category != CategoryUnclassified {
		t.Errorf("got %q, want %q", result.Category, CategoryUnclassified)
	}
	if result.ClassifierName != "none" {
		t.Errorf("got classifier %q, want none", result.ClassifierName)
	}
	svc.Close()
}

func TestService_LLMFallback(t *testing.T) {
	// Create mock LLM that returns a misconception.
	resp := json.RawMessage(`{"misconception_id":"add-no-carry","confidence":0.92,"reasoning":"Added columns without carrying"}`)
	mock := llm.NewMockProvider(llm.MockResponse{Content: resp})

	svc := NewService(mock)

	var mu sync.Mutex
	var asyncResult *DiagnosisResult
	done := make(chan struct{})

	cb := func(r *DiagnosisResult) {
		mu.Lock()
		asyncResult = r
		mu.Unlock()
		close(done)
	}

	// Slow answer, low accuracy → rules don't match → LLM dispatched.
	syncResult := svc.Diagnose(context.Background(), testQuestion(), "715", 5000, 0.40, cb)
	if syncResult.Category != CategoryUnclassified {
		t.Errorf("sync result: got %q, want %q", syncResult.Category, CategoryUnclassified)
	}

	// Wait for async result.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for async LLM diagnosis")
	}

	mu.Lock()
	defer mu.Unlock()

	if asyncResult == nil {
		t.Fatal("async result is nil")
	}
	if asyncResult.Category != CategoryMisconception {
		t.Errorf("async result: got %q, want %q", asyncResult.Category, CategoryMisconception)
	}
	if asyncResult.MisconceptionID != "add-no-carry" {
		t.Errorf("misconception ID = %q, want add-no-carry", asyncResult.MisconceptionID)
	}
	if asyncResult.Confidence != 0.92 {
		t.Errorf("confidence = %f, want 0.92", asyncResult.Confidence)
	}

	svc.Close()
}

func TestService_LLMNoMatch(t *testing.T) {
	// LLM returns null misconception_id.
	resp := json.RawMessage(`{"misconception_id":null,"confidence":0.3,"reasoning":"Could be multiple causes"}`)
	mock := llm.NewMockProvider(llm.MockResponse{Content: resp})

	svc := NewService(mock)

	var mu sync.Mutex
	var asyncResult *DiagnosisResult
	done := make(chan struct{})

	cb := func(r *DiagnosisResult) {
		mu.Lock()
		asyncResult = r
		mu.Unlock()
		close(done)
	}

	svc.Diagnose(context.Background(), testQuestion(), "wrong", 5000, 0.40, cb)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for async LLM diagnosis")
	}

	mu.Lock()
	defer mu.Unlock()

	if asyncResult == nil {
		t.Fatal("async result is nil")
	}
	if asyncResult.Category != CategoryUnclassified {
		t.Errorf("got %q, want %q", asyncResult.Category, CategoryUnclassified)
	}

	svc.Close()
}

func TestService_LLMInvalidID(t *testing.T) {
	// LLM returns an ID not in the candidate list.
	resp := json.RawMessage(`{"misconception_id":"fake-id","confidence":0.8,"reasoning":"test"}`)
	mock := llm.NewMockProvider(llm.MockResponse{Content: resp})

	svc := NewService(mock)

	var mu sync.Mutex
	var asyncResult *DiagnosisResult
	done := make(chan struct{})

	cb := func(r *DiagnosisResult) {
		mu.Lock()
		asyncResult = r
		mu.Unlock()
		close(done)
	}

	svc.Diagnose(context.Background(), testQuestion(), "wrong", 5000, 0.40, cb)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for async LLM diagnosis")
	}

	mu.Lock()
	defer mu.Unlock()

	if asyncResult == nil {
		t.Fatal("async result is nil")
	}
	if asyncResult.Category != CategoryUnclassified {
		t.Errorf("got %q, want %q (invalid ID should be unclassified)", asyncResult.Category, CategoryUnclassified)
	}

	svc.Close()
}

func TestService_SpeedRushPriority_OverLLM(t *testing.T) {
	// Even with LLM available, speed-rush should be detected synchronously.
	mock := llm.NewMockProvider() // Empty queue — shouldn't be called.
	svc := NewService(mock)

	result := svc.Diagnose(context.Background(), testQuestion(), "wrong", 500, 0.40, nil)
	if result.Category != CategorySpeedRush {
		t.Errorf("got %q, want %q", result.Category, CategorySpeedRush)
	}
	if mock.CallCount() != 0 {
		t.Errorf("LLM was called %d times, want 0", mock.CallCount())
	}

	svc.Close()
}
