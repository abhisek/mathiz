package playslot

import (
	"errors"
	"testing"
)

func TestAcquireReleaseAcrossSurfaces(t *testing.T) {
	r := NewRegistry()

	release, err := r.Acquire("child-1", "treasure map")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// Any surface is refused while the slot is held — including the same one.
	var busy ErrBusy
	if _, err := r.Acquire("child-1", "terminal"); !errors.As(err, &busy) || busy.Surface != "treasure map" {
		t.Fatalf("cross-surface acquire: got %v, want ErrBusy{treasure map}", err)
	}
	if _, err := r.Acquire("child-1", "treasure map"); !errors.As(err, &busy) {
		t.Fatalf("same-surface acquire: got %v, want ErrBusy", err)
	}

	// Other children are unaffected.
	rel2, err := r.Acquire("child-2", "terminal")
	if err != nil {
		t.Fatalf("other child: %v", err)
	}
	rel2()

	// Release frees the slot; releasing twice is harmless.
	release()
	release()
	rel3, err := r.Acquire("child-1", "terminal")
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	rel3()
}
