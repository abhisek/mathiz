package termbridge

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/store"
)

type testWorld struct {
	srv   *httptest.Server
	token string
}

func newTestWorld(t *testing.T) *testWorld {
	t.Helper()
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	svc := family.New(st.Client())
	ctx := context.Background()
	acct, _ := svc.EnsureAccount(ctx, "sb-1", "p@example.com", "P")
	sp, _ := svc.CreateSpace(ctx, acct.UID, "Fam")
	child, _ := svc.AddChild(ctx, sp.UID, "Alice", 3, "")
	inv, _ := svc.CreateInvite(ctx, sp.UID, 0)
	token, _, err := svc.RedeemInvite(ctx, inv.Code, child.UID, "", "test-device")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	bridge := New(Options{
		Store:   st,
		Family:  svc,
		Checker: authz.NewChecker(svc),
	})
	srv := httptest.NewServer(bridge)
	t.Cleanup(srv.Close)
	return &testWorld{srv: srv, token: token}
}

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}

func dial(t *testing.T, w *testWorld) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(w.srv.URL), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// readControl reads frames until the next text (control) frame.
func readControl(t *testing.T, conn *websocket.Conn, timeout time.Duration) serverMsg {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		mt, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read control: %v", err)
		}
		if mt != websocket.TextMessage {
			continue
		}
		var msg serverMsg
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatalf("unmarshal control: %v", err)
		}
		return msg
	}
}

func authMsg(token string) []byte {
	b, _ := json.Marshal(clientMsg{Type: "auth", Token: token, Cols: 100, Rows: 30})
	return b
}

func TestSessionLifecycle(t *testing.T) {
	w := newTestWorld(t)
	conn := dial(t, w)

	if err := conn.WriteMessage(websocket.TextMessage, authMsg(w.token)); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	if msg := readControl(t, conn, 5*time.Second); msg.Type != "ready" {
		t.Fatalf("expected ready, got %+v", msg)
	}

	// The TUI renders: expect ANSI bytes on a binary frame.
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	sawOutput := false
	for !sawOutput {
		mt, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read output: %v", err)
		}
		if mt == websocket.BinaryMessage && len(payload) > 0 {
			sawOutput = true
		}
	}

	// Resize is accepted without killing the session.
	resize, _ := json.Marshal(clientMsg{Type: "resize", Cols: 120, Rows: 40})
	if err := conn.WriteMessage(websocket.TextMessage, resize); err != nil {
		t.Fatalf("write resize: %v", err)
	}

	// Ctrl+C quits the app; the server announces exit before closing.
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0x03}); err != nil {
		t.Fatalf("write ctrl+c: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_ = conn.SetReadDeadline(deadline)
		mt, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for exit: %v", err)
		}
		if mt == websocket.TextMessage {
			var msg serverMsg
			_ = json.Unmarshal(payload, &msg)
			if msg.Type == "exit" {
				return
			}
		}
	}
}

func TestRejectsBadToken(t *testing.T) {
	w := newTestWorld(t)
	conn := dial(t, w)

	if err := conn.WriteMessage(websocket.TextMessage, authMsg("mzd_definitely-not-a-real-token")); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	if msg := readControl(t, conn, 5*time.Second); msg.Type != "error" {
		t.Fatalf("expected error, got %+v", msg)
	}
}

func TestRejectsMissingAuth(t *testing.T) {
	w := newTestWorld(t)
	conn := dial(t, w)

	// Binary garbage instead of the auth handshake.
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("junk")); err != nil {
		t.Fatalf("write junk: %v", err)
	}
	if msg := readControl(t, conn, 5*time.Second); msg.Type != "error" {
		t.Fatalf("expected error, got %+v", msg)
	}
}

func TestPerChildExclusivity(t *testing.T) {
	w := newTestWorld(t)

	// First session for the child goes live.
	first := dial(t, w)
	if err := first.WriteMessage(websocket.TextMessage, authMsg(w.token)); err != nil {
		t.Fatalf("auth 1: %v", err)
	}
	if msg := readControl(t, first, 5*time.Second); msg.Type != "ready" {
		t.Fatalf("first session: %+v", msg)
	}

	// A second session for the same child (another tab/device) is refused —
	// it would load a stale snapshot and clobber the first session's save.
	second := dial(t, w)
	if err := second.WriteMessage(websocket.TextMessage, authMsg(w.token)); err != nil {
		t.Fatalf("auth 2: %v", err)
	}
	if msg := readControl(t, second, 5*time.Second); msg.Type != "error" {
		t.Fatalf("expected error for concurrent same-child session, got %+v", msg)
	}
}

func TestSessionCap(t *testing.T) {
	// A bridge with MaxSessions=1: the first session occupies the slot,
	// the second is turned away.
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	svc := family.New(st.Client())
	ctx := context.Background()
	acct, _ := svc.EnsureAccount(ctx, "sb-2", "p2@example.com", "P")
	sp, _ := svc.CreateSpace(ctx, acct.UID, "Fam2")
	child, _ := svc.AddChild(ctx, sp.UID, "Bob", 4, "")
	inv, _ := svc.CreateInvite(ctx, sp.UID, 0)
	token, _, _ := svc.RedeemInvite(ctx, inv.Code, child.UID, "", "d")

	bridge := New(Options{Store: st, Family: svc, Checker: authz.NewChecker(svc), MaxSessions: 1})
	srv := httptest.NewServer(bridge)
	t.Cleanup(srv.Close)

	first, _, err := websocket.DefaultDialer.Dial(wsURL(srv.URL), nil)
	if err != nil {
		t.Fatalf("dial 1: %v", err)
	}
	t.Cleanup(func() { first.Close() })
	if err := first.WriteMessage(websocket.TextMessage, authMsg(token)); err != nil {
		t.Fatalf("auth 1: %v", err)
	}
	if msg := readControl(t, first, 5*time.Second); msg.Type != "ready" {
		t.Fatalf("first session: %+v", msg)
	}

	second, _, err := websocket.DefaultDialer.Dial(wsURL(srv.URL), nil)
	if err != nil {
		t.Fatalf("dial 2: %v", err)
	}
	t.Cleanup(func() { second.Close() })
	if err := second.WriteMessage(websocket.TextMessage, authMsg(token)); err != nil {
		t.Fatalf("auth 2: %v", err)
	}
	if msg := readControl(t, second, 5*time.Second); msg.Type != "error" {
		t.Fatalf("expected error for capped session, got %+v", msg)
	}
}
