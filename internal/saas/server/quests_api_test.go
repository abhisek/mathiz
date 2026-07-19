package server

import (
	"testing"

	"github.com/abhisek/mathiz/internal/saas/credits"
)

// questTestFixture bootstraps family A (with a child + device token) and a
// second parent B for cross-tenant probes.
type questTestFixture struct {
	env        *testEnv
	parentA    string
	parentB    string
	spaceID    string
	childID    string
	childToken string
}

func newQuestFixture(t *testing.T) *questTestFixture {
	t.Helper()
	e := newTestEnv(t)
	f := &questTestFixture{
		env:     e,
		parentA: parentToken(t, "sb-parent-a", "a@example.com"),
		parentB: parentToken(t, "sb-parent-b", "b@example.com"),
	}

	var space spaceJSON
	resp := e.call(t, "POST", "/api/v1/family", f.parentA, map[string]string{"name": "The As"}, &space)
	expectStatus(t, resp, 201, "create family")
	f.spaceID = space.ID

	// Parent B gets a family too (so its requests are well-formed parents).
	e.call(t, "POST", "/api/v1/family", f.parentB, map[string]string{"name": "The Bs"}, nil)

	var child childJSON
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/children", f.parentA,
		map[string]any{"name": "Alice", "grade": 3}, &child)
	expectStatus(t, resp, 201, "add child")
	f.childID = child.ID

	var invite inviteJSON
	e.call(t, "POST", "/api/v1/family/"+space.ID+"/invites", f.parentA, map[string]any{}, &invite)
	var redeemed struct {
		Token string `json:"token"`
	}
	resp = e.call(t, "POST", "/api/v1/join/redeem", "",
		map[string]string{"code": invite.Code, "childProfileId": child.ID, "deviceLabel": "iPad"}, &redeemed)
	expectStatus(t, resp, 200, "redeem")
	f.childToken = redeemed.Token
	return f
}

type questAPIQuestion struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Generated bool   `json:"generated"`
}

func TestQuestAPILifecycle(t *testing.T) {
	f := newQuestFixture(t)
	e := f.env

	// Create a draft quest.
	var quest questJSON
	resp := e.call(t, "POST", "/api/v1/family/"+f.spaceID+"/quests", f.parentA,
		map[string]any{"name": "HCF week", "emoji": "🧮", "childId": f.childID}, &quest)
	expectStatus(t, resp, 201, "create quest")
	if quest.Status != "draft" || quest.ChildID != f.childID {
		t.Fatalf("quest = %+v", quest)
	}

	// Cross-tenant probes get 404, not 403 — on every quest route.
	for _, probe := range []struct{ method, path string }{
		{"GET", "/api/v1/family/" + f.spaceID + "/quests"},
		{"POST", "/api/v1/family/" + f.spaceID + "/quests"},
		{"GET", "/api/v1/quests/" + quest.ID},
		{"PATCH", "/api/v1/quests/" + quest.ID},
		{"DELETE", "/api/v1/quests/" + quest.ID},
		{"POST", "/api/v1/quests/" + quest.ID + "/questions"},
		{"POST", "/api/v1/quests/" + quest.ID + "/generate"},
		{"POST", "/api/v1/quests/" + quest.ID + "/publish"},
	} {
		body := map[string]any{}
		resp := e.call(t, probe.method, probe.path, f.parentB, body, nil)
		expectStatus(t, resp, 404, "cross-tenant "+probe.method+" "+probe.path)
	}

	// Publishing an empty quest is refused.
	resp = e.call(t, "POST", "/api/v1/quests/"+quest.ID+"/publish", f.parentA, map[string]any{}, nil)
	expectStatus(t, resp, 422, "publish empty quest")

	// Manual authoring: a typo'd answer saves WITH a warning.
	var saved struct {
		Question questAPIQuestion `json:"question"`
		Warning  string           `json:"warning"`
	}
	resp = e.call(t, "POST", "/api/v1/quests/"+quest.ID+"/questions", f.parentA, map[string]any{
		"text": "What is 2 + 2?", "answer": "5", "answerType": "integer", "format": "numeric",
	}, &saved)
	expectStatus(t, resp, 201, "add typo question")
	if saved.Warning == "" {
		t.Error("expected a mathcheck warning for 2 + 2 = 5")
	}
	// Fix it: warning clears.
	resp = e.call(t, "PATCH", "/api/v1/quests/"+quest.ID+"/questions/"+saved.Question.ID, f.parentA, map[string]any{
		"text": "What is 2 + 2?", "answer": "4", "answerType": "integer", "format": "numeric",
	}, &saved)
	expectStatus(t, resp, 200, "fix question")
	if saved.Warning != "" {
		t.Errorf("warning after fix = %q", saved.Warning)
	}

	// AI generation adds reviewed-marked drafts and debits the wallet
	// (the starter grant covers it).
	var gen struct {
		Questions []questAPIQuestion `json:"questions"`
		Replayed  bool               `json:"replayed"`
	}
	resp = e.call(t, "POST", "/api/v1/quests/"+quest.ID+"/generate", f.parentA,
		map[string]any{"brief": "one addition problem", "count": 1, "clientKey": "click-1"}, &gen)
	expectStatus(t, resp, 200, "generate")
	if len(gen.Questions) != 1 || !gen.Questions[0].Generated || gen.Replayed {
		t.Fatalf("generate = %+v", gen)
	}

	// Quest detail shows both questions.
	var detail struct {
		Quest     questJSON          `json:"quest"`
		Questions []questAPIQuestion `json:"questions"`
	}
	resp = e.call(t, "GET", "/api/v1/quests/"+quest.ID, f.parentA, nil, &detail)
	expectStatus(t, resp, 200, "quest detail")
	if detail.Quest.QuestionCount != 2 || len(detail.Questions) != 2 {
		t.Fatalf("detail = %+v", detail)
	}

	// Publish; the kid's map now carries the quest card.
	resp = e.call(t, "POST", "/api/v1/quests/"+quest.ID+"/publish", f.parentA, map[string]any{}, &quest)
	expectStatus(t, resp, 200, "publish")
	if quest.Status != "active" {
		t.Fatalf("published quest = %+v", quest)
	}

	var mapView struct {
		Quests []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Total   int    `json:"total"`
			Correct int    `json:"correct"`
			Done    bool   `json:"done"`
		} `json:"quests"`
	}
	resp = e.call(t, "GET", "/api/v1/game/map", f.childToken, nil, &mapView)
	expectStatus(t, resp, 200, "map")
	if len(mapView.Quests) != 1 || mapView.Quests[0].ID != quest.ID || mapView.Quests[0].Total != 2 {
		t.Fatalf("map quests = %+v", mapView.Quests)
	}

	// The kid starts a quest expedition and answers through it.
	var exp struct {
		ID             string `json:"id"`
		QuestID        string `json:"questId"`
		TotalQuestions int    `json:"totalQuestions"`
	}
	resp = e.call(t, "POST", "/api/v1/game/quests/"+quest.ID+"/expeditions", f.childToken, map[string]any{}, &exp)
	expectStatus(t, resp, 201, "start quest expedition")
	if exp.QuestID != quest.ID || exp.TotalQuestions != 2 {
		t.Fatalf("expedition = %+v", exp)
	}
	var question struct {
		Text string `json:"text"`
	}
	resp = e.call(t, "POST", "/api/v1/game/expeditions/"+exp.ID+"/question", f.childToken, nil, &question)
	expectStatus(t, resp, 200, "quest question")
	if question.Text != "What is 2 + 2?" {
		t.Fatalf("question = %+v", question)
	}
	var answer struct {
		Correct bool `json:"correct"`
		Done    bool `json:"done"`
		Summary *struct {
			QuestID       string `json:"questId"`
			QuestComplete bool   `json:"questComplete"`
		} `json:"summary"`
	}
	resp = e.call(t, "POST", "/api/v1/game/expeditions/"+exp.ID+"/answer", f.childToken,
		map[string]string{"answer": "4"}, &answer)
	expectStatus(t, resp, 200, "answer 1")
	if !answer.Correct || answer.Done {
		t.Fatalf("answer 1 = %+v", answer)
	}
	e.call(t, "POST", "/api/v1/game/expeditions/"+exp.ID+"/question", f.childToken, nil, &question)
	resp = e.call(t, "POST", "/api/v1/game/expeditions/"+exp.ID+"/answer", f.childToken,
		map[string]string{"answer": "20"}, &answer)
	expectStatus(t, resp, 200, "answer 2")
	if !answer.Done || answer.Summary == nil || !answer.Summary.QuestComplete {
		t.Fatalf("final answer = %+v", answer)
	}

	// The map card flips to done; a finished quest can't restart.
	resp = e.call(t, "GET", "/api/v1/game/map", f.childToken, nil, &mapView)
	expectStatus(t, resp, 200, "map after quest")
	if !mapView.Quests[0].Done || mapView.Quests[0].Correct != 2 {
		t.Fatalf("map quests after = %+v", mapView.Quests)
	}
	resp = e.call(t, "POST", "/api/v1/game/quests/"+quest.ID+"/expeditions", f.childToken, map[string]any{}, nil)
	expectStatus(t, resp, 409, "restart finished quest")
}

func TestQuestKidRoutesAreScoped(t *testing.T) {
	f := newQuestFixture(t)
	e := f.env

	// An active quest in family A, targeted at everyone.
	var quest questJSON
	e.call(t, "POST", "/api/v1/family/"+f.spaceID+"/quests", f.parentA,
		map[string]any{"name": "Quest"}, &quest)
	e.call(t, "POST", "/api/v1/quests/"+quest.ID+"/questions", f.parentA, map[string]any{
		"text": "What is 3 + 3?", "answer": "6", "answerType": "integer", "format": "numeric",
	}, nil)
	e.call(t, "POST", "/api/v1/quests/"+quest.ID+"/publish", f.parentA, map[string]any{}, nil)

	// A kid from family B cannot start it: 404, existence not confirmed.
	spaceBID := mustSpaceID(t, e, f.parentB)
	var childB childJSON
	resp := e.call(t, "POST", "/api/v1/family/"+spaceBID+"/children", f.parentB,
		map[string]any{"name": "Bob", "grade": 4}, &childB)
	expectStatus(t, resp, 201, "add child B")
	var inviteB inviteJSON
	e.call(t, "POST", "/api/v1/family/"+spaceBID+"/invites", f.parentB, map[string]any{}, &inviteB)
	var redeemedB struct {
		Token string `json:"token"`
	}
	e.call(t, "POST", "/api/v1/join/redeem", "",
		map[string]string{"code": inviteB.Code, "childProfileId": childB.ID, "deviceLabel": "d"}, &redeemedB)

	resp = e.call(t, "POST", "/api/v1/game/quests/"+quest.ID+"/expeditions", redeemedB.Token, map[string]any{}, nil)
	expectStatus(t, resp, 404, "cross-family quest start")

	// Family B's map shows no quests from family A.
	var mapView struct {
		Quests []any `json:"quests"`
	}
	resp = e.call(t, "GET", "/api/v1/game/map", redeemedB.Token, nil, &mapView)
	expectStatus(t, resp, 200, "map B")
	if len(mapView.Quests) != 0 {
		t.Fatalf("family B sees family A's quests: %+v", mapView.Quests)
	}

	// Archiving hides the quest from family A's own kid too.
	e.call(t, "PATCH", "/api/v1/quests/"+quest.ID, f.parentA, map[string]any{"status": "archived"}, nil)
	resp = e.call(t, "POST", "/api/v1/game/quests/"+quest.ID+"/expeditions", f.childToken, map[string]any{}, nil)
	expectStatus(t, resp, 404, "archived quest start")
}

func TestQuestGenerateOutOfCredits(t *testing.T) {
	f := newQuestFixture(t)
	e := f.env

	var quest questJSON
	e.call(t, "POST", "/api/v1/family/"+f.spaceID+"/quests", f.parentA,
		map[string]any{"name": "Broke quest"}, &quest)

	// Drain the starter grant, then generation must 402 with the standard
	// out_of_credits marker (and save nothing).
	creditsSvc := credits.New(e.st.Client())
	if err := creditsSvc.Debit(t.Context(), f.spaceID, credits.StarterCredits, "test:drain"); err != nil {
		t.Fatalf("drain: %v", err)
	}
	var body errorBody
	resp := e.call(t, "POST", "/api/v1/quests/"+quest.ID+"/generate", f.parentA,
		map[string]any{"brief": "anything", "count": 5, "clientKey": "click-1"}, &body)
	expectStatus(t, resp, 402, "generate broke")
	if body.Error != "out_of_credits" {
		t.Fatalf("error = %q", body.Error)
	}
	var detail struct {
		Questions []questAPIQuestion `json:"questions"`
	}
	resp = e.call(t, "GET", "/api/v1/quests/"+quest.ID, f.parentA, nil, &detail)
	expectStatus(t, resp, 200, "detail")
	if len(detail.Questions) != 0 {
		t.Fatalf("questions saved despite 402: %+v", detail.Questions)
	}
}

// mustSpaceID fetches the parent's family space ID via /me.
func mustSpaceID(t *testing.T, e *testEnv, token string) string {
	t.Helper()
	var me struct {
		Family *spaceJSON `json:"family"`
	}
	resp := e.call(t, "GET", "/api/v1/me", token, nil, &me)
	expectStatus(t, resp, 200, "me")
	if me.Family == nil {
		t.Fatal("parent has no family")
	}
	return me.Family.ID
}
