package problemgen

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/abhisek/mathiz/internal/llm"
)

// Source says where a question handed out by Prefetcher.Get came from.
type Source string

const (
	SourcePrefetchHit  Source = "prefetch_hit"  // warmed and ready when asked for
	SourcePrefetchJoin Source = "prefetch_join" // warm still in flight, awaited
	SourceFresh        Source = "fresh"         // generated on request
)

// Result is one question from Prefetcher.Get.
type Result struct {
	Question *Question
	Source   Source
	Attempts int // provider attempts the question took, retries included
	// Generation identifies the generation that produced (or failed) this
	// result. Concurrent callers sharing one generation see the same value,
	// so a caller can count one shared failure once.
	Generation uint64
}

// ErrPrefetcherClosed is returned by Get after Close.
var ErrPrefetcherClosed = errors.New("prefetcher closed")

// PrefetchConfig configures a Prefetcher.
type PrefetchConfig struct {
	// Budget bounds every generation, warm or fresh, from its start.
	// Zero means no deadline beyond the Generator's own.
	Budget time.Duration
	// DisableWarm turns Warm into a no-op (the MATHIZ_QUESTION_PREFETCH=0
	// off switch). Get still de-duplicates concurrent identical requests.
	DisableWarm bool
	// LogAttrs label the background outcome log lines (e.g. child_uid).
	LogAttrs []any
}

// PrefetchDisabledByEnv reports the MATHIZ_QUESTION_PREFETCH=0 off switch,
// read by both surfaces so they can't disagree.
func PrefetchDisabledByEnv() bool {
	return os.Getenv("MATHIZ_QUESTION_PREFETCH") == "0"
}

// Prefetcher generates the next question while the kid is still reading
// feedback (specs/19 §5). It holds at most one question, keyed by a
// fingerprint of every GenerateInput field: a warmed question is handed out
// only for an identical input, so no caller has to reason about staleness.
//
// Every generation runs on the prefetcher's own context plus the budget,
// never on a request's context: a caller that gives up (a closed tab) must
// not cancel a generation that another caller, or the next tap, waits on.
// The mutex is never held while waiting on a generator, and replacing or
// closing only cancels, never joins, so Warm and Close are safe to call
// under a caller's own lock. The Generator must honour ctx cancellation
// (LLMGenerator does, through the provider), or a cancelled call runs on.
type Prefetcher struct {
	gen  Generator
	cfg  PrefetchConfig
	base context.Context
	stop context.CancelFunc

	mu      sync.Mutex
	entry   *prefetchEntry
	closed  bool
	lastGen uint64 // last generation number handed out (guarded by mu)
}

type prefetchEntry struct {
	gen     uint64
	fp      [sha256.Size]byte
	warm    bool // started by Warm rather than by Get
	started time.Time
	cancel  context.CancelFunc
	done    chan struct{}

	// Written by the generating goroutine before done closes.
	q        *Question
	err      error
	attempts int

	used       bool // claimed by a Get (guarded by Prefetcher.mu)
	usedLogged bool // the "used" outcome was logged (guarded by Prefetcher.mu)
}

// NewPrefetcher wraps gen. Close it when the session ends.
func NewPrefetcher(gen Generator, cfg PrefetchConfig) *Prefetcher {
	base, stop := context.WithCancel(context.Background())
	return &Prefetcher{gen: gen, cfg: cfg, base: base, stop: stop}
}

// Warm starts generating input in the background unless an identical
// request is already in flight, ready, or was just handed out. A different
// input replaces (cancels) the held one.
func (p *Prefetcher) Warm(input GenerateInput) {
	if p.cfg.DisableWarm {
		return
	}
	fp := fingerprint(input)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || (p.entry != nil && p.entry.fp == fp) {
		return
	}
	p.dropLocked("input_changed")
	p.entry = p.startLocked(fp, input, true)
}

// Get returns the question for input: the held one when the fingerprint
// matches (waiting for it if still in flight), otherwise a fresh one. ctx
// bounds only this caller's wait; on ctx's end the generation carries on,
// and a later Get with the same input can still use it. A warm that failed,
// or any entry cancelled under the caller (replaced or closed), is retried
// once; after Close that retry reports ErrPrefetcherClosed.
func (p *Prefetcher) Get(ctx context.Context, input GenerateInput) (Result, error) {
	fp := fingerprint(input)
	for retried := false; ; retried = true {
		e, source, err := p.claim(fp, input)
		if err != nil {
			return Result{}, err
		}
		select {
		case <-e.done:
		case <-ctx.Done():
			return Result{Source: source}, ctx.Err()
		}
		if e.err == nil {
			p.markUsed(e)
			return Result{Question: e.q, Source: source, Attempts: e.attempts, Generation: e.gen}, nil
		}
		p.forget(e)
		if retried || (!e.warm && !errors.Is(e.err, context.Canceled)) {
			return Result{Source: source, Attempts: e.attempts, Generation: e.gen}, e.err
		}
	}
}

// claim returns the entry Get should wait on, starting a fresh one when the
// held entry is for another input or has failed.
func (p *Prefetcher) claim(fp [sha256.Size]byte, input GenerateInput) (*prefetchEntry, Source, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, "", ErrPrefetcherClosed
	}
	if e := p.entry; e != nil && e.fp == fp {
		select {
		case <-e.done:
			if e.err == nil {
				e.used = true
				return e, sourceFor(e, SourcePrefetchHit), nil
			}
		default:
			// Claimed now, so a replacement before it lands isn't logged
			// as an unused warm.
			e.used = true
			return e, sourceFor(e, SourcePrefetchJoin), nil
		}
	}
	p.dropLocked("input_changed")
	p.entry = p.startLocked(fp, input, false)
	p.entry.used = true
	return p.entry, SourceFresh, nil
}

// sourceFor reports how a caller got e: a fresh entry another caller
// started (a double tap) is still a fresh generation.
func sourceFor(e *prefetchEntry, warmSource Source) Source {
	if e.warm {
		return warmSource
	}
	return SourceFresh
}

func (p *Prefetcher) markUsed(e *prefetchEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e.warm && !e.usedLogged {
		p.logOutcome(e, "used", "")
	}
	e.usedLogged = true
}

// forget drops a failed entry so the next Get starts over.
func (p *Prefetcher) forget(e *prefetchEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.entry == e {
		p.entry = nil
	}
}

// Close cancels in-flight work and drops the held question. Idempotent.
func (p *Prefetcher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	p.dropLocked("expedition_over")
	p.stop()
}

// startLocked launches one generation. Caller holds p.mu.
func (p *Prefetcher) startLocked(fp [sha256.Size]byte, input GenerateInput, warm bool) *prefetchEntry {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	if p.cfg.Budget > 0 {
		ctx, cancel = context.WithTimeout(p.base, p.cfg.Budget)
	} else {
		ctx, cancel = context.WithCancel(p.base)
	}
	ctx, attempts := llm.WithAttemptCounter(ctx)
	p.lastGen++
	e := &prefetchEntry{gen: p.lastGen, fp: fp, warm: warm, started: time.Now(), cancel: cancel, done: make(chan struct{})}
	input = input.clone()
	go func() {
		defer cancel()
		q, err := p.gen.Generate(ctx, input)
		e.q, e.err, e.attempts = q, err, attempts.Count()
		close(e.done)
		// A cancel (replaced or closed) was already logged by dropLocked;
		// running out of budget is a failure like any other.
		if err != nil && warm && !errors.Is(err, context.Canceled) {
			p.logOutcome(e, "failed", err.Error())
		}
	}()
	return e
}

// dropLocked releases the held entry: an unfinished one is cancelled, and
// a warmed question nobody used is logged as wasted. Caller holds p.mu.
func (p *Prefetcher) dropLocked(reason string) {
	e := p.entry
	if e == nil {
		return
	}
	p.entry = nil
	select {
	case <-e.done:
		if e.warm && !e.used && e.err == nil {
			p.logOutcome(e, "discarded", reason)
		}
	default:
		e.cancel()
		if e.warm {
			p.logOutcome(e, "cancelled", reason)
		}
	}
}

func (p *Prefetcher) logOutcome(e *prefetchEntry, outcome, reason string) {
	attrs := append(slices.Clone(p.cfg.LogAttrs),
		"outcome", outcome, "reason", reason, "ms", time.Since(e.started).Milliseconds())
	slog.Info("question prefetch", attrs...)
}

// clone copies the slices, so a generation running in the background never
// shares memory with the caller's session state. Skill is not deep-copied:
// skills come from the immutable skill graph.
func (in GenerateInput) clone() GenerateInput {
	in.PriorQuestions = slices.Clone(in.PriorQuestions)
	in.RecentErrors = slices.Clone(in.RecentErrors)
	return in
}

// fingerprint hashes every GenerateInput field the question depends on.
// Each field is length-prefixed, so ["ab","c"] and ["a","bc"] differ. The
// skill is identified by ID alone: skills come from the skill graph, where
// the ID determines every other field.
func fingerprint(in GenerateInput) [sha256.Size]byte {
	var buf []byte
	writeStr := func(s string) {
		buf = binary.BigEndian.AppendUint64(buf, uint64(len(s)))
		buf = append(buf, s...)
	}
	writeList := func(list []string) {
		buf = binary.BigEndian.AppendUint64(buf, uint64(len(list)))
		for _, s := range list {
			writeStr(s)
		}
	}
	writeStr(in.Skill.ID)
	buf = binary.BigEndian.AppendUint64(buf, uint64(in.Tier))
	writeList(in.PriorQuestions)
	writeList(in.RecentErrors)
	writeStr(in.LearnerProfile)
	return sha256.Sum256(buf)
}
