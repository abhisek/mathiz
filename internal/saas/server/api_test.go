package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/saas/activity"
	"github.com/abhisek/mathiz/internal/saas/auth"
	"github.com/abhisek/mathiz/internal/saas/billing"
	"github.com/abhisek/mathiz/internal/saas/credits"
	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/saas/game"
	"github.com/abhisek/mathiz/internal/saas/quests"
	"github.com/abhisek/mathiz/internal/store"
)

// testGenBatchJSON is the mock LLM's canned quest-generation batch.
const testGenBatchJSON = `{"questions":[
	{"question_text":"What is 12 + 8?","format":"numeric","answer":"20","answer_type":"integer","choices":[],"hint":"Add the ones first.","difficulty":2,"explanation":"12 + 8 = 20."}
]}`

// stubGenerator serves deterministic questions for game expeditions.
type stubGenerator struct{}

func (stubGenerator) Generate(_ context.Context, input problemgen.GenerateInput) (*problemgen.Question, error) {
	return &problemgen.Question{
		Text:        "What is 2 + 2?",
		Format:      problemgen.FormatNumeric,
		Answer:      "4",
		AnswerType:  problemgen.AnswerTypeInteger,
		Explanation: "2 and 2 make 4.",
		SkillID:     input.Skill.ID,
		Tier:        input.Tier,
	}, nil
}

const testJWTSecret = "test-secret-value-with-enough-length!!"

type testEnv struct {
	ts *httptest.Server
	st *store.Store
}

func newTestEnv(t *testing.T) *testEnv {
	return newTestEnvWith(t, nil)
}

// newTestEnvWith lets a test tweak the server config before wiring.
func newTestEnvWith(t *testing.T, mutate func(*Config)) *testEnv {
	t.Helper()
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	verifier, err := auth.NewSupabaseVerifier(auth.SupabaseConfig{JWTSecret: testJWTSecret})
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	cfg := &Config{
		Addr:            ":0",
		DatabaseURL:     "test",
		SupabaseURL:     "https://example.supabase.co",
		SupabaseAnonKey: "anon-key",
	}
	if mutate != nil {
		mutate(cfg)
	}
	creditsSvc := credits.New(st.Client())
	// Quests + game are wired with deterministic fakes: a mock LLM for
	// question generation and a fake per-expedition generator, so quest and
	// game routes are exercisable without a network.
	questsSvc := quests.New(st.Client(), creditsSvc, func(ctx context.Context) (llm.Provider, error) {
		return llm.NewMockProvider(llm.MockResponse{Content: []byte(testGenBatchJSON)}), nil
	})
	gameMgr := game.NewManager(game.Config{
		Store: st,
		Toolset: func(ctx context.Context, eventRepo store.EventRepo) (*game.Toolset, error) {
			return &game.Toolset{Generator: stubGenerator{}}, nil
		},
		Quests: questsSvc,
	})
	familySvc := family.New(st.Client())
	activityReader := activity.NewReader(st, questsSvc, func(ctx context.Context, accountID string) (string, error) {
		a, err := familySvc.Account(ctx, accountID)
		if err != nil {
			return "", err
		}
		if a.DisplayName != "" {
			return a.DisplayName, nil
		}
		return a.Email, nil
	})
	srv := New(Deps{
		Config:   cfg,
		Store:    st,
		Family:   familySvc,
		Verifier: verifier,
		Credits:  creditsSvc,
		Billing:  billing.NewService(st.Client(), creditsSvc, billing.NewFakeProvider(cfg.PublicBaseURL)),
		Game:     gameMgr,
		Quests:   questsSvc,
		Activity: activityReader,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &testEnv{ts: ts, st: st}
}

// parentToken mints a valid Supabase-style HS256 token for a test user.
func parentToken(t *testing.T, sub, email string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   sub,
		"aud":   "authenticated",
		"email": email,
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

// call makes a JSON request and decodes the response into out (if non-nil).
func (e *testEnv) call(t *testing.T, method, path, bearer string, body any, out any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, e.ts.URL+path, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return resp
}

func expectStatus(t *testing.T, resp *http.Response, want int, label string) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("%s: status = %d, want %d", label, resp.StatusCode, want)
	}
}

func TestBootConfigIsPublic(t *testing.T) {
	e := newTestEnv(t)
	var cfg map[string]string
	resp := e.call(t, "GET", "/api/v1/config", "", nil, &cfg)
	expectStatus(t, resp, 200, "config")
	if cfg["supabaseUrl"] != "https://example.supabase.co" || cfg["supabaseAnonKey"] != "anon-key" {
		t.Errorf("config = %v", cfg)
	}
	// No PostHog key configured → the posthog fields are OMITTED entirely,
	// so a key-less deployment (self-hosters, tests) ships zero analytics.
	for _, k := range []string{"posthogKey", "posthogHost"} {
		if _, ok := cfg[k]; ok {
			t.Errorf("config leaks %q with analytics off: %v", k, cfg)
		}
	}
}

func TestBootConfigServesPostHogWhenConfigured(t *testing.T) {
	e := newTestEnvWith(t, func(cfg *Config) {
		cfg.PostHogAPIKey = "phc_test"
		cfg.PostHogHost = "https://eu.i.posthog.com"
	})
	var cfg map[string]string
	resp := e.call(t, "GET", "/api/v1/config", "", nil, &cfg)
	expectStatus(t, resp, 200, "config")
	// posthogHost is the same-origin relay path — the upstream host
	// (cfg.PostHogHost) must never reach the browser.
	if cfg["posthogKey"] != "phc_test" || cfg["posthogHost"] != "/relay" {
		t.Errorf("config = %v", cfg)
	}
}

func TestParentEndpointsRequireAuth(t *testing.T) {
	e := newTestEnv(t)
	for _, path := range []string{"/api/v1/me", "/api/v1/family/x/children"} {
		resp := e.call(t, "GET", path, "", nil, nil)
		expectStatus(t, resp, 401, "no token "+path)
	}
	resp := e.call(t, "GET", "/api/v1/me", "garbage-token", nil, nil)
	expectStatus(t, resp, 401, "bad token")
}

// TestFullOnboardingFlow drives the golden path: parent signs in → creates
// family → adds child → mints code → child redeems → learns → parent sees
// stats — plus cross-tenant denials along the way.
func TestFullOnboardingFlow(t *testing.T) {
	e := newTestEnv(t)
	parentA := parentToken(t, "sb-parent-a", "a@example.com")
	parentB := parentToken(t, "sb-parent-b", "b@example.com")

	// First contact auto-provisions the account; no family yet.
	var me struct {
		Account accountJSON `json:"account"`
		Family  *spaceJSON  `json:"family"`
	}
	resp := e.call(t, "GET", "/api/v1/me", parentA, nil, &me)
	expectStatus(t, resp, 200, "me")
	if me.Account.Email != "a@example.com" || me.Family != nil {
		t.Fatalf("me = %+v", me)
	}

	// Create the family space.
	var space spaceJSON
	resp = e.call(t, "POST", "/api/v1/family", parentA, map[string]string{"name": "The As"}, &space)
	expectStatus(t, resp, 201, "create family")

	// Second space is rejected.
	resp = e.call(t, "POST", "/api/v1/family", parentA, map[string]string{"name": "Again"}, nil)
	expectStatus(t, resp, 409, "duplicate family")

	// Add a child with a PIN.
	var child childJSON
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/children", parentA,
		map[string]any{"name": "Alice", "grade": 3, "pin": "1234"}, &child)
	expectStatus(t, resp, 201, "add child")
	if !child.HasPIN {
		t.Error("child should report hasPin")
	}

	// Parent B cannot see or touch A's family (404, not 403).
	resp = e.call(t, "GET", "/api/v1/family/"+space.ID+"/children", parentB, nil, nil)
	expectStatus(t, resp, 404, "cross-tenant children list")
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/children", parentB,
		map[string]any{"name": "Mallory", "grade": 3}, nil)
	expectStatus(t, resp, 404, "cross-tenant add child")

	// Mint a join code.
	var invite inviteJSON
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/invites", parentA, map[string]any{}, &invite)
	expectStatus(t, resp, 201, "create invite")

	// Child previews the code without auth.
	var preview struct {
		FamilyName string      `json:"familyName"`
		Children   []childJSON `json:"children"`
	}
	resp = e.call(t, "POST", "/api/v1/join/preview", "", map[string]string{"code": invite.Code}, &preview)
	expectStatus(t, resp, 200, "join preview")
	if preview.FamilyName != "The As" || len(preview.Children) != 1 {
		t.Fatalf("preview = %+v", preview)
	}

	// Redeem needs the right PIN.
	resp = e.call(t, "POST", "/api/v1/join/redeem", "",
		map[string]string{"code": invite.Code, "childProfileId": child.ID, "pin": "9999", "deviceLabel": "iPad"}, nil)
	expectStatus(t, resp, 422, "wrong pin")

	var redeemed struct {
		Token    string    `json:"token"`
		Child    childJSON `json:"child"`
		FamilyID string    `json:"familyId"`
	}
	resp = e.call(t, "POST", "/api/v1/join/redeem", "",
		map[string]string{"code": invite.Code, "childProfileId": child.ID, "pin": "1234", "deviceLabel": "iPad"}, &redeemed)
	expectStatus(t, resp, 200, "redeem")
	if redeemed.FamilyID != space.ID {
		t.Fatalf("redeem familyId = %q, want %q", redeemed.FamilyID, space.ID)
	}

	// The device token authenticates the child.
	var childMe struct {
		Profile    childJSON `json:"profile"`
		FamilyName string    `json:"familyName"`
		FamilyID   string    `json:"familyId"`
	}
	resp = e.call(t, "GET", "/api/v1/child/me", redeemed.Token, nil, &childMe)
	expectStatus(t, resp, 200, "child me")
	if childMe.Profile.ID != child.ID || childMe.FamilyName != "The As" || childMe.FamilyID != space.ID {
		t.Fatalf("child me = %+v", childMe)
	}

	// Simulate learning: events land in the child's owner-scoped stream.
	eventRepo := e.st.EventRepoFor(child.ID)
	if err := eventRepo.AppendSessionEvent(t.Context(), store.SessionEventData{
		SessionID: "s1", Action: "end", QuestionsServed: 6, CorrectAnswers: 5, DurationSecs: 300,
	}); err != nil {
		t.Fatalf("append session: %v", err)
	}

	// Parent A sees the child's stats; parent B gets 404.
	var stats struct {
		RecentSessions []map[string]any `json:"recentSessions"`
	}
	resp = e.call(t, "GET", "/api/v1/children/"+child.ID+"/stats", parentA, nil, &stats)
	expectStatus(t, resp, 200, "stats")
	if len(stats.RecentSessions) != 1 {
		t.Fatalf("recent sessions = %d, want 1", len(stats.RecentSessions))
	}
	resp = e.call(t, "GET", "/api/v1/children/"+child.ID+"/stats", parentB, nil, nil)
	expectStatus(t, resp, 404, "cross-tenant stats")

	// Devices: list, cross-tenant revoke denied, owner revoke works.
	var devices struct {
		Devices []deviceJSON `json:"devices"`
	}
	resp = e.call(t, "GET", "/api/v1/children/"+child.ID+"/devices", parentA, nil, &devices)
	expectStatus(t, resp, 200, "devices")
	if len(devices.Devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(devices.Devices))
	}
	deviceID := devices.Devices[0].ID
	resp = e.call(t, "DELETE", "/api/v1/devices/"+deviceID, parentB, nil, nil)
	expectStatus(t, resp, 404, "cross-tenant device revoke")
	resp = e.call(t, "DELETE", "/api/v1/devices/"+deviceID, parentA, nil, nil)
	expectStatus(t, resp, 204, "device revoke")

	// Revoked token no longer authenticates.
	resp = e.call(t, "GET", "/api/v1/child/me", redeemed.Token, nil, nil)
	expectStatus(t, resp, 401, "revoked device")

	// Invite can be revoked; preview then fails.
	resp = e.call(t, "DELETE", "/api/v1/invites/"+invite.ID, parentA, nil, nil)
	expectStatus(t, resp, 204, "invite revoke")
	resp = e.call(t, "POST", "/api/v1/join/preview", "", map[string]string{"code": invite.Code}, nil)
	expectStatus(t, resp, 422, "revoked invite preview")
}

func TestChildTokenCannotUseParentEndpoints(t *testing.T) {
	e := newTestEnv(t)
	parentA := parentToken(t, "sb-parent-a", "a@example.com")

	var space spaceJSON
	e.call(t, "POST", "/api/v1/family", parentA, map[string]string{"name": "Fam"}, &space)
	var child childJSON
	e.call(t, "POST", "/api/v1/family/"+space.ID+"/children", parentA,
		map[string]any{"name": "Kid", "grade": 4}, &child)
	var invite inviteJSON
	e.call(t, "POST", "/api/v1/family/"+space.ID+"/invites", parentA, map[string]any{}, &invite)
	var redeemed struct {
		Token string `json:"token"`
	}
	e.call(t, "POST", "/api/v1/join/redeem", "",
		map[string]string{"code": invite.Code, "childProfileId": child.ID, "deviceLabel": "d"}, &redeemed)

	// A device token is not a Supabase JWT: parent endpoints reject it.
	resp := e.call(t, "GET", "/api/v1/me", redeemed.Token, nil, nil)
	expectStatus(t, resp, 401, "device token on parent endpoint")
	resp = e.call(t, "GET", "/api/v1/children/"+child.ID+"/stats", redeemed.Token, nil, nil)
	expectStatus(t, resp, 401, "device token on stats")
}

func TestUpdateChildAndArchive(t *testing.T) {
	e := newTestEnv(t)
	parentA := parentToken(t, "sb-parent-a", "a@example.com")

	var space spaceJSON
	e.call(t, "POST", "/api/v1/family", parentA, map[string]string{"name": "Fam"}, &space)
	var child childJSON
	e.call(t, "POST", "/api/v1/family/"+space.ID+"/children", parentA,
		map[string]any{"name": "Kid", "grade": 4}, &child)

	// Rename + set a PIN.
	var updated childJSON
	resp := e.call(t, "PATCH", "/api/v1/children/"+child.ID, parentA,
		map[string]any{"name": "Kiddo", "pin": "5678"}, &updated)
	expectStatus(t, resp, 200, "update child")
	if updated.Name != "Kiddo" || !updated.HasPIN {
		t.Fatalf("updated = %+v", updated)
	}

	// Bad grade rejected.
	resp = e.call(t, "PATCH", "/api/v1/children/"+child.ID, parentA,
		map[string]any{"grade": 12}, nil)
	expectStatus(t, resp, 400, "bad grade")

	// Archive removes from roster.
	resp = e.call(t, "PATCH", "/api/v1/children/"+child.ID, parentA,
		map[string]any{"archived": true}, nil)
	expectStatus(t, resp, 200, "archive")
	var list struct {
		Children []json.RawMessage `json:"children"`
	}
	resp = e.call(t, "GET", "/api/v1/family/"+space.ID+"/children", parentA, nil, &list)
	expectStatus(t, resp, 200, "list after archive")
	if len(list.Children) != 0 {
		t.Errorf("children = %d, want 0", len(list.Children))
	}
}

func TestJoinRedeemUnknownFieldsRejected(t *testing.T) {
	e := newTestEnv(t)
	resp := e.call(t, "POST", "/api/v1/join/preview", "", map[string]any{"code": "X", "extra": true}, nil)
	expectStatus(t, resp, 400, "unknown fields")
}
