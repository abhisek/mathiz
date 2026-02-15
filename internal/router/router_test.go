package router

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/abhisek/mathiz/internal/screen"
)

// stubScreen is a minimal screen for testing.
type stubScreen struct {
	title   string
	initRan bool
}

func (s *stubScreen) Init() tea.Cmd {
	s.initRan = true
	return nil
}
func (s *stubScreen) Update(tea.Msg) (screen.Screen, tea.Cmd) { return s, nil }
func (s *stubScreen) View(int, int) string                    { return s.title }
func (s *stubScreen) Title() string                           { return s.title }

func TestPush(t *testing.T) {
	s1 := &stubScreen{title: "first"}
	r := New(s1)

	s2 := &stubScreen{title: "second"}
	r.Push(s2)

	if r.Depth() != 2 {
		t.Errorf("expected depth 2, got %d", r.Depth())
	}
	if r.Active().Title() != "second" {
		t.Errorf("expected active 'second', got %q", r.Active().Title())
	}
	if !s2.initRan {
		t.Error("expected Init() to run on pushed screen")
	}
}

func TestPop(t *testing.T) {
	s1 := &stubScreen{title: "first"}
	r := New(s1)

	s2 := &stubScreen{title: "second"}
	r.Push(s2)
	r.Pop()

	if r.Depth() != 1 {
		t.Errorf("expected depth 1, got %d", r.Depth())
	}
	if r.Active().Title() != "first" {
		t.Errorf("expected active 'first', got %q", r.Active().Title())
	}
}

func TestPopNoopAtBottom(t *testing.T) {
	s1 := &stubScreen{title: "first"}
	r := New(s1)

	r.Pop()

	if r.Depth() != 1 {
		t.Errorf("expected depth 1 after pop at bottom, got %d", r.Depth())
	}
}

func TestReplace(t *testing.T) {
	s1 := &stubScreen{title: "first"}
	r := New(s1)

	s2 := &stubScreen{title: "second"}
	r.Replace(s2)

	if r.Depth() != 1 {
		t.Errorf("expected depth 1 after replace, got %d", r.Depth())
	}
	if r.Active().Title() != "second" {
		t.Errorf("expected active 'second', got %q", r.Active().Title())
	}
	if !s2.initRan {
		t.Error("expected Init() to run on replaced screen")
	}
}

func TestReplaceScreenMsg(t *testing.T) {
	s1 := &stubScreen{title: "first"}
	r := New(s1)

	s2 := &stubScreen{title: "second"}
	r.Update(ReplaceScreenMsg{Screen: s2})

	if r.Active().Title() != "second" {
		t.Errorf("expected active 'second', got %q", r.Active().Title())
	}
	if !s2.initRan {
		t.Error("expected Init() to run via ReplaceScreenMsg")
	}
}

func TestReplacePreservesStackDepth(t *testing.T) {
	s1 := &stubScreen{title: "first"}
	r := New(s1)

	s2 := &stubScreen{title: "second"}
	r.Push(s2)

	s3 := &stubScreen{title: "third"}
	r.Replace(s3)

	if r.Depth() != 2 {
		t.Errorf("expected depth 2, got %d", r.Depth())
	}
	if r.Active().Title() != "third" {
		t.Errorf("expected active 'third', got %q", r.Active().Title())
	}
}
