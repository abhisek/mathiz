// Package logctx carries a per-request bag of structured log attributes.
//
// The server's request-logging middleware installs an empty bag into the
// request context; anything downstream (auth middlewares, handlers, error
// helpers) appends attributes via Add. When the request completes, the
// middleware emits ONE canonical log line containing every attribute added
// along the way — errors and annotations never need their own log lines.
package logctx

import (
	"context"
	"log/slog"
	"sync"
)

type ctxKey struct{}

// bag is a mutex-guarded attribute list. The pointer lives in the context,
// so attributes added after context propagation are still visible at emit
// time. The mutex matters: handlers may annotate from spawned goroutines.
type bag struct {
	mu    sync.Mutex
	attrs []slog.Attr
}

// Install returns a context carrying a fresh, empty attribute bag.
func Install(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKey{}, &bag{})
}

// Add appends an attribute to the request's bag. A context without a bag
// (tests, background jobs) is a silent no-op — annotating is best-effort.
func Add(ctx context.Context, key string, value any) {
	b, ok := ctx.Value(ctxKey{}).(*bag)
	if !ok {
		return
	}
	b.mu.Lock()
	b.attrs = append(b.attrs, slog.Any(key, value))
	b.mu.Unlock()
}

// Attrs returns a snapshot of the attributes added so far.
func Attrs(ctx context.Context) []slog.Attr {
	b, ok := ctx.Value(ctxKey{}).(*bag)
	if !ok {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]slog.Attr, len(b.attrs))
	copy(out, b.attrs)
	return out
}
