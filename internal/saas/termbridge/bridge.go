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
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/gorilla/websocket"

	"github.com/abhisek/mathiz/internal/app"
	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/gems"
	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/store"
)

// Options configures the bridge.
type Options struct {
	Store   *store.Store
	Family  *family.Service
	Checker *authz.Checker

	// IdleTimeout disconnects sessions with no client input. Zero disables.
	IdleTimeout time.Duration

	// MaxSessions caps concurrent sessions. Zero means 100.
	MaxSessions int
}

// Bridge is the WebSocket handler for learning sessions.
type Bridge struct {
	opts   Options
	active atomic.Int64

	upgrader websocket.Upgrader
}

func New(opts Options) *Bridge {
	if opts.MaxSessions <= 0 {
		opts.MaxSessions = 100
	}
	return &Bridge{
		opts: opts,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// The SPA is served same-origin; cross-origin browser calls are
			// rejected. Non-browser clients (no Origin header) are allowed —
			// they hold a bearer token, which is the actual credential.
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				return origin == "" || sameHost(origin, r.Host)
			},
		},
	}
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

	if int(b.active.Load()) >= b.opts.MaxSessions {
		s.fail("server is full, try again in a few minutes")
		return
	}

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
	principal := authz.Principal{
		Kind:           authz.KindChild,
		ChildProfileID: child.UID,
		FamilySpaceID:  child.FamilySpaceID,
	}
	if err := b.opts.Checker.CanLearnAs(ctx, principal, child.UID); err != nil {
		s.fail("not allowed")
		return
	}

	cols, rows := hello.Cols, hello.Rows
	if cols <= 0 || rows <= 0 {
		cols, rows = 80, 24
	}

	b.active.Add(1)
	defer b.active.Add(-1)

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

// buildAppOptions mirrors cmd/run.go's dependency wiring with owner-scoped
// repositories. The returned cleanup releases per-session resources.
func buildAppOptions(ctx context.Context, st *store.Store, ownerID string) (app.Options, func()) {
	eventRepo := st.EventRepoFor(ownerID)
	opts := app.Options{
		EventRepo:    eventRepo,
		SnapshotRepo: st.SnapshotRepoFor(ownerID),
		GemService:   gems.NewService(eventRepo),
	}

	cleanup := func() {}
	// Per-session provider so LLM usage events land in this child's stream.
	provider, err := llm.NewProviderFromEnv(ctx, eventRepo)
	if err == nil {
		opts.LLMProvider = provider
		opts.Generator = problemgen.New(provider, problemgen.DefaultConfig())
		diagService := diagnosis.NewService(provider)
		opts.DiagnosisService = diagService
		opts.LessonService = lessons.NewService(provider, lessons.DefaultConfig())
		opts.Compressor = lessons.NewCompressor(provider, lessons.DefaultCompressorConfig())
		cleanup = func() { diagService.Close() }
	}
	return opts, cleanup
}

// sameHost reports whether an Origin header points at the given host.
func sameHost(origin, host string) bool {
	for _, scheme := range []string{"http://", "https://"} {
		if origin == scheme+host {
			return true
		}
	}
	return false
}
