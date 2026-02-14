package router

import (
	"github.com/abhisek/mathiz/internal/screen"

	tea "charm.land/bubbletea/v2"
)

// PushScreenMsg requests the router to push a new screen onto the stack.
type PushScreenMsg struct {
	Screen screen.Screen
}

// PopScreenMsg requests the router to pop the current screen off the stack.
type PopScreenMsg struct{}

// Router manages a stack of screens.
type Router struct {
	stack []screen.Screen
}

// New creates a new Router with the given initial screen.
func New(initial screen.Screen) *Router {
	return &Router{
		stack: []screen.Screen{initial},
	}
}

// Push adds a screen on top of the stack and calls its Init().
func (r *Router) Push(s screen.Screen) tea.Cmd {
	r.stack = append(r.stack, s)
	return s.Init()
}

// Pop removes the top screen. No-op if stack depth would become 0.
func (r *Router) Pop() tea.Cmd {
	if len(r.stack) <= 1 {
		return nil
	}
	r.stack = r.stack[:len(r.stack)-1]
	return nil
}

// Active returns the top screen on the stack.
func (r *Router) Active() screen.Screen {
	if len(r.stack) == 0 {
		return nil
	}
	return r.stack[len(r.stack)-1]
}

// Depth returns the number of screens on the stack.
func (r *Router) Depth() int {
	return len(r.stack)
}

// Update forwards a message to the active screen and handles navigation messages.
func (r *Router) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case PushScreenMsg:
		return r.Push(msg.Screen)
	case PopScreenMsg:
		return r.Pop()
	}

	active := r.Active()
	if active == nil {
		return nil
	}

	updated, cmd := active.Update(msg)
	r.stack[len(r.stack)-1] = updated
	return cmd
}

// View renders the active screen.
func (r *Router) View(width, height int) string {
	active := r.Active()
	if active == nil {
		return ""
	}
	return active.View(width, height)
}
