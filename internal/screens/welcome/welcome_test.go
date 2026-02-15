package welcome

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/abhisek/mathiz/internal/router"
	"github.com/abhisek/mathiz/internal/screen"
)

// stubScreen is a minimal screen implementation for testing.
type stubScreen struct{}

func (s *stubScreen) Init() tea.Cmd                          { return nil }
func (s *stubScreen) Update(tea.Msg) (screen.Screen, tea.Cmd) { return s, nil }
func (s *stubScreen) View(int, int) string                   { return "home" }
func (s *stubScreen) Title() string                          { return "Home" }

func newTestWelcome() (*WelcomeScreen, int) {
	callCount := 0
	factory := func() screen.Screen {
		callCount++
		return &stubScreen{}
	}
	return New(factory), callCount
}

func newTestWelcomeWithCounter() (*WelcomeScreen, *int) {
	callCount := 0
	factory := func() screen.Screen {
		callCount++
		return &stubScreen{}
	}
	return New(factory), &callCount
}

func sendTicks(w *WelcomeScreen, n int) (screen.Screen, tea.Cmd) {
	var s screen.Screen = w
	var cmd tea.Cmd
	for i := 0; i < n; i++ {
		s, cmd = s.Update(tickMsg(time.Now()))
	}
	return s, cmd
}

func TestPhaseTransitions(t *testing.T) {
	w, _ := newTestWelcomeWithCounter()

	// Initially at phase 0 — no banner visible
	view := w.View(80, 24)
	if containsBanner(view) {
		t.Error("banner should not be visible at start")
	}

	// After 5 ticks (500ms) — phase 1 complete, sparkles should start
	sendTicks(w, 5)
	if w.elapsed != 500*time.Millisecond {
		t.Errorf("expected elapsed 500ms, got %v", w.elapsed)
	}

	// After 15 ticks (1500ms) — phase 2 complete
	sendTicks(w, 10)
	if w.elapsed != 1500*time.Millisecond {
		t.Errorf("expected elapsed 1500ms, got %v", w.elapsed)
	}

	// After 25 ticks (2500ms) — phase 3 complete, banner should be visible
	sendTicks(w, 10)
	view = w.View(80, 24)
	if !containsBanner(view) {
		t.Error("banner should be visible after phase 3")
	}
}

func TestKeypressDuringAnimationSkipsToTransition(t *testing.T) {
	w, callCount := newTestWelcomeWithCounter()

	// Advance a bit so we're mid-animation
	sendTicks(w, 3)

	_, cmd := w.Update(tea.KeyPressMsg{Code: ' '})
	if cmd == nil {
		t.Fatal("keypress during animation should trigger transition")
	}
	msg := cmd()
	if _, ok := msg.(router.ReplaceScreenMsg); !ok {
		t.Fatalf("expected ReplaceScreenMsg, got %T", msg)
	}
	if *callCount != 1 {
		t.Errorf("factory should be called once, got %d", *callCount)
	}
}

func TestKeypressAfterAnimationEmitsReplace(t *testing.T) {
	w, callCount := newTestWelcomeWithCounter()

	// Complete the animation
	sendTicks(w, 45)

	_, cmd := w.Update(tea.KeyPressMsg{Code: ' '})
	if cmd == nil {
		t.Fatal("expected a command from keypress after animation")
	}

	msg := cmd()
	replaceMsg, ok := msg.(router.ReplaceScreenMsg)
	if !ok {
		t.Fatalf("expected ReplaceScreenMsg, got %T", msg)
	}
	if replaceMsg.Screen == nil {
		t.Error("replace screen should not be nil")
	}
	if *callCount != 1 {
		t.Errorf("factory should be called once, got %d", *callCount)
	}
}

func TestNoAutoTransition(t *testing.T) {
	w, callCount := newTestWelcomeWithCounter()

	// Send 45 ticks to complete animation — ticks keep going (for sparkle animation)
	// but factory should not be called without a keypress
	sendTicks(w, 45)
	if *callCount != 0 {
		t.Errorf("factory should not be called without keypress, got %d", *callCount)
	}
	// Elapsed should be capped at totalDur
	if w.elapsed != totalDur {
		t.Errorf("expected elapsed capped at %v, got %v", totalDur, w.elapsed)
	}
}

func TestFactoryCalledOnce(t *testing.T) {
	w, callCount := newTestWelcomeWithCounter()

	// Complete animation, then trigger transition via keypress
	sendTicks(w, 45)
	w.Update(tea.KeyPressMsg{Code: 'a'})

	// Try again — should not call factory again
	_, cmd := w.Update(tea.KeyPressMsg{Code: 'b'})
	if cmd != nil {
		t.Error("second keypress should not produce a command")
	}
	if *callCount != 1 {
		t.Errorf("factory should be called exactly once, got %d", *callCount)
	}
}

func TestTitleEmpty(t *testing.T) {
	w, _ := newTestWelcomeWithCounter()
	if w.Title() != "" {
		t.Errorf("expected empty title, got %q", w.Title())
	}
}

func containsBanner(s string) bool {
	// Check for the tagline that appears with the banner
	return contains(s, "make math fun")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
