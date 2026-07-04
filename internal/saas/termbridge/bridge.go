// Package termbridge streams the unmodified Mathiz TUI to a browser
// terminal. One WebSocket connection = one Bubble Tea program running
// server-side with owner-scoped repositories.
//
// Protocol (client ⇄ server):
//
//	client → server  text  {"type":"auth","token":"mzd_...","cols":80,"rows":24}
//	server → client  text  {"type":"ready"} | {"type":"error","message":...} | {"type":"exit"}
//	client → server  text  {"type":"resize","cols":100,"rows":30}
//	client → server  binary  raw terminal input bytes (xterm.js onData)
//	server → client  binary  ANSI output from the renderer
//
// The token travels in the first message, never in the URL: query strings
// end up in access logs.
package termbridge

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/gorilla/websocket"

	"github.com/abhisek/mathiz/internal/app"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/store"
)

// Options configures the bridge.
type Options struct {
	Store   *store.Store
	Family  *family.Service
	Checker *authz.Checker

	// AllowedOrigins are extra Origins permitted to open terminal sessions,
	// for split SPA deployments. Same-hostname origins are always allowed.
	AllowedOrigins []string

	// IdleTimeout disconnects sessions with no client input. Zero disables.
	IdleTimeout time.Duration

	// MaxSessions caps concurrent sessions. Zero means 100.
	MaxSessions int
}

// Bridge is the WebSocket handler for learning sessions.
type Bridge struct {
	opts   Options
	active atomic.Int64

	// playing tracks which children have a live session, so a second tab or
	// device can't run concurrently and clobber the first session's snapshot.
	playing sync.Map // child profile UID → struct{}

	upgrader websocket.Upgrader
}

func New(opts Options) *Bridge {
	if opts.MaxSessions <= 0 {
		opts.MaxSessions = 100
	}
	b := &Bridge{opts: opts}
	b.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     b.originAllowed,
	}
	return b
}

// originAllowed is the browser CSRF guard for the upgrade. Allowed: no
// Origin (non-browser clients — the device token is the real credential),
// exact same-origin, a configured allowlist entry, and same-hostname with a
// different port (the Vite dev proxy and single-box reverse proxies rewrite
// Host but not Origin).
func (b *Bridge) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "http://"+r.Host || origin == "https://"+r.Host {
		return true
	}
	for _, allowed := range b.opts.AllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	if u, err := url.Parse(origin); err == nil {
		requestHost := r.Host
		if h, _, err := net.SplitHostPort(requestHost); err == nil {
			requestHost = h
		}
		if h := u.Hostname(); h != "" && h == requestHost {
			return true
		}
	}
	return false
}

// ActiveSessions reports the number of live terminal sessions.
func (b *Bridge) ActiveSessions() int { return int(b.active.Load()) }

type clientMsg struct {
	Type  string `json:"type"`
	Token string `json:"token,omitempty"`
	Cols  int    `json:"cols,omitempty"`
	Rows  int    `json:"rows,omitempty"`
}

type serverMsg struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
}

func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := b.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the HTTP error
	}
	sess := &session{bridge: b, conn: conn}
	sess.run(r.Context())
}

// session is one live terminal connection.
type session struct {
	bridge *Bridge
	conn   *websocket.Conn

	writeMu sync.Mutex
}

// writeControl sends a JSON control message (text frame).
func (s *session) writeControl(msg serverMsg) error {
	payload, _ := json.Marshal(msg)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, payload)
}

// Write implements io.Writer for the Bubble Tea renderer: every chunk of
// ANSI output becomes one binary frame.
func (s *session) Write(p []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *session) fail(msg string) {
	_ = s.writeControl(serverMsg{Type: "error", Message: msg})
	_ = s.conn.Close()
}

func (s *session) run(ctx context.Context) {
	defer s.conn.Close()
	b := s.bridge

	// Reserve a session slot atomically (check-then-act would let a burst of
	// concurrent handshakes blow past the cap while none had incremented yet).
	if n := b.active.Add(1); n > int64(b.opts.MaxSessions) {
		b.active.Add(-1)
		s.fail("server is full, try again in a few minutes")
		return
	}
	defer b.active.Add(-1)

	// First message must authenticate within a short window.
	_ = s.conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var hello clientMsg
	if mt, payload, err := s.conn.ReadMessage(); err != nil || mt != websocket.TextMessage {
		s.fail("expected auth message")
		return
	} else if err := json.Unmarshal(payload, &hello); err != nil || hello.Type != "auth" {
		s.fail("expected auth message")
		return
	}
	_ = s.conn.SetReadDeadline(time.Time{})

	_, child, err := b.opts.Family.ResolveDeviceToken(ctx, hello.Token)
	if err != nil {
		s.fail("invalid credentials")
		return
	}
	principal := authz.ChildPrincipal(child)
	if err := b.opts.Checker.CanLearnAs(ctx, principal, child.UID); err != nil {
		s.fail("not allowed")
		return
	}

	// One live session per child: a second tab/device would load a stale
	// snapshot and clobber the first session's progress on save.
	if _, alreadyPlaying := b.playing.LoadOrStore(child.UID, struct{}{}); alreadyPlaying {
		s.fail("Looks like you're already playing on another screen! Close it first.")
		return
	}
	defer b.playing.Delete(child.UID)

	cols, rows := hello.Cols, hello.Rows
	if cols <= 0 || rows <= 0 {
		cols, rows = 80, 24
	}

	if err := s.writeControl(serverMsg{Type: "ready"}); err != nil {
		return
	}

	sessCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The learner's whole world is owner-scoped to their profile ID.
	opts, cleanup := buildAppOptions(sessCtx, b.opts.Store, child.UID)
	defer cleanup()

	inputR, inputW := io.Pipe()
	defer inputW.Close()

	p := tea.NewProgram(app.NewModel(opts),
		tea.WithContext(sessCtx),
		tea.WithInput(inputR),
		tea.WithOutput(s),
		tea.WithEnvironment([]string{"TERM=xterm-256color", "COLORTERM=truecolor"}),
		tea.WithColorProfile(colorprofile.TrueColor),
		tea.WithWindowSize(cols, rows),
		tea.WithoutSignals(),
	)

	// Terminate via context cancellation, never p.Kill() from another
	// goroutine: killing a program mid-startup races its initialization,
	// while the program watches its context at all times.

	// Idle watchdog: any client activity feeds it.
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())
	if b.opts.IdleTimeout > 0 {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-sessCtx.Done():
					return
				case <-ticker.C:
					idle := time.Since(time.Unix(0, lastActivity.Load()))
					if idle > b.opts.IdleTimeout {
						log.Printf("termbridge: closing idle session for child %s", child.UID)
						cancel()
						return
					}
				}
			}
		}()
	}

	// Reader: client frames → program input / resize / quit.
	go func() {
		defer inputW.Close()
		for {
			mt, payload, err := s.conn.ReadMessage()
			if err != nil {
				cancel()
				return
			}
			lastActivity.Store(time.Now().UnixNano())
			switch mt {
			case websocket.BinaryMessage:
				if _, err := inputW.Write(payload); err != nil {
					return
				}
			case websocket.TextMessage:
				var msg clientMsg
				if err := json.Unmarshal(payload, &msg); err != nil {
					continue
				}
				if msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0 {
					p.Send(tea.WindowSizeMsg{Width: msg.Cols, Height: msg.Rows})
				}
			}
		}
	}()

	if _, err := p.Run(); err != nil && sessCtx.Err() == nil {
		log.Printf("termbridge: session for child %s ended: %v", child.UID, err)
	}
	_ = s.writeControl(serverMsg{Type: "exit"})
}

// buildAppOptions assembles the standard app wiring (shared with cmd/run.go
// via app.BuildOptions) on owner-scoped repositories. The per-session LLM
// provider logs usage events into this child's stream.
func buildAppOptions(ctx context.Context, st *store.Store, ownerID string) (app.Options, func()) {
	eventRepo := st.EventRepoFor(ownerID)
	provider, err := llm.NewProviderFromEnv(ctx, eventRepo)
	if err != nil {
		provider = nil // AI features off; the TUI degrades gracefully
	}
	return app.BuildOptions(eventRepo, st.SnapshotRepoFor(ownerID), provider)
}
