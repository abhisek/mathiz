package server

import (
	"testing"

	"github.com/abhisek/mathiz/internal/store"
)

// activityFixture builds a family with owner + co-parent + stranger and two
// children, seeds child A's stream, and returns everything the tests need.
type activityFixture struct {
	e                         *testEnv
	owner, coParent, stranger string
	childA, childB            childJSON
}

func newActivityFixture(t *testing.T) *activityFixture {
	t.Helper()
	e := newTestEnv(t)
	f := &activityFixture{
		e:        e,
		owner:    parentToken(t, "sb-owner", "owner@example.com"),
		coParent: parentToken(t, "sb-cop", "cop@example.com"),
		stranger: parentToken(t, "sb-stranger", "stranger@example.com"),
	}

	var space spaceJSON
	resp := e.call(t, "POST", "/api/v1/family", f.owner, map[string]string{"name": "Fam"}, &space)
	expectStatus(t, resp, 201, "create family")
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/children", f.owner,
		map[string]any{"name": "Ada", "grade": 3}, &f.childA)
	expectStatus(t, resp, 201, "add child A")
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/children", f.owner,
		map[string]any{"name": "Ben", "grade": 4}, &f.childB)
	expectStatus(t, resp, 201, "add child B")

	// Co-parent joins.
	var invite parentInviteJSON
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/parents", f.owner,
		map[string]string{"email": "cop@example.com"}, &invite)
	expectStatus(t, resp, 201, "invite co-parent")
	resp = e.call(t, "POST", "/api/v1/invites/parent/"+invite.ID+"/accept", f.coParent, nil, nil)
	expectStatus(t, resp, 200, "accept invite")

	// The stranger has their own family so it's a real cross-tenant probe.
	resp = e.call(t, "POST", "/api/v1/family", f.stranger, map[string]string{"name": "Other"}, nil)
	expectStatus(t, resp, 201, "stranger family")

	// Child A learns: one finished session with answers, a hint, a mastery.
	repo := e.st.EventRepoFor(f.childA.ID)
	ctx := t.Context()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	must(repo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID: "sess-a", Action: "start",
		PlanSummary: []store.PlanSlotSummaryData{{SkillID: "pv-hundreds", Tier: "learn", Category: "frontier"}},
	}))
	must(repo.AppendAnswerEvent(ctx, store.AnswerEventData{
		SessionID: "sess-a", SkillID: "pv-hundreds", Tier: "learn", Category: "frontier",
		QuestionText: "2+2?", CorrectAnswer: "4", LearnerAnswer: "4",
		Correct: true, TimeMs: 1500, AnswerFormat: "integer",
	}))
	must(repo.AppendHintEvent(ctx, store.HintEventData{
		SessionID: "sess-a", SkillID: "pv-hundreds", QuestionText: "2+2?", HintText: "count",
	}))
	must(repo.AppendMasteryEvent(ctx, store.MasteryEventData{
		SkillID: "pv-hundreds", FromState: "learning", ToState: "mastered",
		Trigger: "fluency", FluencyScore: 0.9, SessionID: "sess-a",
	}))
	must(repo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID: "sess-a", Action: "end", QuestionsServed: 1, CorrectAnswers: 1, DurationSecs: 90,
	}))
	return f
}

type activityPageJSON struct {
	Items []struct {
		Kind       string              `json:"kind"`
		Seq        int64               `json:"seq"`
		At         string              `json:"at"`
		Expedition *expeditionItemJSON `json:"expedition"`
		Mastery    *masteryItemJSON    `json:"mastery"`
		Lesson     *lessonItemJSON     `json:"lesson"`
	} `json:"items"`
	NextBefore *int64 `json:"nextBefore"`
}

func TestActivityEndpointsAuthzMatrix(t *testing.T) {
	f := newActivityFixture(t)
	e := f.e

	paths := []string{
		"/api/v1/children/" + f.childA.ID + "/activity",
		"/api/v1/children/" + f.childA.ID + "/activity/sessions/sess-a",
	}
	for _, path := range paths {
		resp := e.call(t, "GET", path, f.owner, nil, nil)
		expectStatus(t, resp, 200, "owner "+path)
		resp = e.call(t, "GET", path, f.coParent, nil, nil)
		expectStatus(t, resp, 200, "co-parent "+path)
		resp = e.call(t, "GET", path, f.stranger, nil, nil)
		expectStatus(t, resp, 404, "stranger "+path)
		resp = e.call(t, "GET", path, "", nil, nil)
		expectStatus(t, resp, 401, "unauthenticated "+path)
	}
}

func TestActivityTimelineResponseShape(t *testing.T) {
	f := newActivityFixture(t)

	var page activityPageJSON
	resp := f.e.call(t, "GET", "/api/v1/children/"+f.childA.ID+"/activity", f.owner, nil, &page)
	expectStatus(t, resp, 200, "activity")
	if len(page.Items) != 2 {
		t.Fatalf("items = %d, want 2 (expedition + mastery)", len(page.Items))
	}
	if page.NextBefore != nil {
		t.Errorf("nextBefore = %v, want omitted on short page", *page.NextBefore)
	}
	exp := page.Items[0]
	if exp.Kind != "expedition" || exp.Expedition == nil || exp.Seq == 0 || exp.At == "" {
		t.Fatalf("expedition item = %+v", exp)
	}
	if exp.Expedition.SessionID != "sess-a" || exp.Expedition.Questions != 1 ||
		exp.Expedition.Correct != 1 || exp.Expedition.DurationSecs != 90 {
		t.Errorf("expedition payload = %+v", exp.Expedition)
	}
	if exp.Expedition.Category != "frontier" {
		t.Errorf("expedition category = %q, want frontier (first plan slot)", exp.Expedition.Category)
	}
	if len(exp.Expedition.Skills) != 1 || exp.Expedition.Skills[0].ID != "pv-hundreds" ||
		exp.Expedition.Skills[0].Name != "Place Value to 1,000" {
		t.Errorf("expedition skills = %+v", exp.Expedition.Skills)
	}
	if exp.Expedition.Quest != nil {
		t.Errorf("quest = %+v, want omitted for a normal dig", exp.Expedition.Quest)
	}
	mas := page.Items[1]
	if mas.Kind != "mastery" || mas.Mastery == nil ||
		mas.Mastery.SkillID != "pv-hundreds" || mas.Mastery.ToState != "mastered" ||
		mas.Mastery.FromState != "learning" {
		t.Fatalf("mastery item = %+v payload %+v", mas, mas.Mastery)
	}

	// Kind filter + pagination params round-trip.
	page = activityPageJSON{}
	resp = f.e.call(t, "GET", "/api/v1/children/"+f.childA.ID+"/activity?kinds=mastery&limit=1",
		f.owner, nil, &page)
	expectStatus(t, resp, 200, "filtered activity")
	if len(page.Items) != 1 || page.Items[0].Kind != "mastery" {
		t.Fatalf("filtered items = %+v", page.Items)
	}
	if page.NextBefore == nil || *page.NextBefore != page.Items[0].Seq {
		t.Errorf("nextBefore = %v, want %d on a full page", page.NextBefore, page.Items[0].Seq)
	}

	// Bad params are 400, not 500.
	resp = f.e.call(t, "GET", "/api/v1/children/"+f.childA.ID+"/activity?kinds=gems", f.owner, nil, nil)
	expectStatus(t, resp, 400, "unknown kind")
	resp = f.e.call(t, "GET", "/api/v1/children/"+f.childA.ID+"/activity?before=abc", f.owner, nil, nil)
	expectStatus(t, resp, 400, "bad cursor")
	resp = f.e.call(t, "GET", "/api/v1/children/"+f.childA.ID+"/activity?from=yesterday", f.owner, nil, nil)
	expectStatus(t, resp, 400, "bad from")
}

func TestActivitySessionDetail(t *testing.T) {
	f := newActivityFixture(t)

	var detail struct {
		Answers   []answerDetailJSON `json:"answers"`
		HintCount int                `json:"hintCount"`
	}
	resp := f.e.call(t, "GET", "/api/v1/children/"+f.childA.ID+"/activity/sessions/sess-a",
		f.owner, nil, &detail)
	expectStatus(t, resp, 200, "session detail")
	if detail.HintCount != 1 || len(detail.Answers) != 1 {
		t.Fatalf("detail = %+v", detail)
	}
	a := detail.Answers[0]
	if a.SkillID != "pv-hundreds" || a.SkillName != "Place Value to 1,000" ||
		a.QuestionText != "2+2?" || a.LearnerAnswer != "4" || a.CorrectAnswer != "4" ||
		!a.Correct || a.TimeMs != 1500 || a.Seq == 0 || a.At == "" {
		t.Errorf("answer = %+v", a)
	}

	// Unknown session under a managed child → 404.
	resp = f.e.call(t, "GET", "/api/v1/children/"+f.childA.ID+"/activity/sessions/nope",
		f.owner, nil, nil)
	expectStatus(t, resp, 404, "unknown session")

	// Child A's session requested under sibling B (also managed) → 404: the
	// session isn't in B's stream and its existence must not leak.
	resp = f.e.call(t, "GET", "/api/v1/children/"+f.childB.ID+"/activity/sessions/sess-a",
		f.owner, nil, nil)
	expectStatus(t, resp, 404, "cross-child session")
}
