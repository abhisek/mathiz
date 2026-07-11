// Package playslot enforces ONE live learning session per child across
// every surface (treasure-map expeditions, the browser terminal, and any
// future client). Concurrent sessions load the same owner's snapshot and
// clobber each other's mastery/spaced-rep progress on save — per-surface
// maps can't see each other, so the slot must be claimed here.
package playslot

import (
	"fmt"
	"sync"
)

// ErrBusy means another session holds the child's slot.
type ErrBusy struct {
	Surface string // which surface holds it, for logs/messages
}

func (e ErrBusy) Error() string {
	return fmt.Sprintf("already playing on %s", e.Surface)
}

// Registry is the single chokepoint for acquiring a child's play slot.
type Registry struct {
	mu   sync.Mutex
	held map[string]string // child profile UID → surface name
}

func NewRegistry() *Registry {
	return &Registry{held: make(map[string]string)}
}

// Acquire claims the child's slot for a surface, failing with ErrBusy while
// ANY surface holds it. The returned release is idempotent and must be
// called only after the session's final snapshot save — releasing earlier
// reopens the clobber window this package exists to close.
func (r *Registry) Acquire(childUID, surface string) (release func(), err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.held[childUID]; ok {
		return nil, ErrBusy{Surface: s}
	}
	r.held[childUID] = surface
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.held, childUID)
			r.mu.Unlock()
		})
	}, nil
}
