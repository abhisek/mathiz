package quests

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/credits"
	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/saas/game"
	"github.com/abhisek/mathiz/internal/store"
)

// questEnv is a full in-memory control plane: two families, one child each.
type questEnv struct {
	client  *ent.Client
	family  *family.Service
	credits *credits.Service

	acctA, spaceA, childA string
	acctB, spaceB, childB string
}

func newQuestEnv(t *testing.T) *questEnv {
	t.Helper()
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	e := &questEnv{client: st.Client(), family: family.New(st.Client()), credits: credits.New(st.Client())}
	ctx := context.Background()
	bootstrap := func(sb, name string) (string, string, string) {
		acct, err := e.family.EnsureAccount(ctx, sb, sb+"@example.com", name)
		if err != nil {
			t.Fatalf("account: %v", err)
		}
		sp, err := e.family.CreateSpace(ctx, acct.UID, name)
		if err != nil {
			t.Fatalf("space: %v", err)
		}
		child, err := e.family.AddChild(ctx, sp.UID, "Kid of "+name, 3, "")
		if err != nil {
			t.Fatalf("child: %v", err)
		}
		return acct.UID, sp.UID, child.UID
	}
	e.acctA, e.spaceA, e.childA = bootstrap("sb-a", "Family A")
	e.acctB, e.spaceB, e.childB = bootstrap("sb-b", "Family B")
	return e
}

// mockProviderFactory returns a factory serving the given canned responses.
func mockProviderFactory(responses ...llm.MockResponse) ProviderFactory {
	provider := llm.NewMockProvider(responses...)
	return func(ctx context.Context) (llm.Provider, error) { return provider, nil }
}

// goodBatchJSON is a valid 2-question generation payload plus one invalid
// question (empty explanation) that validation must drop.
const goodBatchJSON = `{"questions":[
	{"question_text":"What is 12 + 8?","format":"numeric","answer":"20","answer_type":"integer","choices":[],"hint":"Add the ones first.","difficulty":2,"explanation":"12 + 8 = 20."},
	{"question_text":"What is 9 * 3?","format":"numeric","answer":"27","answer_type":"integer","choices":[],"hint":"Nine, three times.","difficulty":2,"explanation":"9 * 3 = 27."},
	{"question_text":"broken","format":"numeric","answer":"5","answer_type":"integer","choices":[],"hint":"","difficulty":2,"explanation":""}
]}`

func numericQuestion(text, answer string) QuestionInput {
	return QuestionInput{
		Text:        text,
		Answer:      answer,
		AnswerType:  "integer",
		Format:      "numeric",
		Hint:        "count carefully",
		Explanation: "worked solution",
	}
}

func TestQuestCRUDAndValidation(t *testing.T) {
	e := newQuestEnv(t)
	svc := New(e.client, nil, nil)
	ctx := context.Background()

	// Validation on create.
	if _, err := svc.Create(ctx, e.spaceA, "", QuestInput{Name: "  "}); !errors.Is(err, ErrBadName) {
		t.Errorf("empty name: %v", err)
	}
	if _, err := svc.Create(ctx, e.spaceA, "", QuestInput{Name: "Q", SkillID: "no-such-skill"}); !errors.Is(err, ErrBadSkill) {
		t.Errorf("bad skill: %v", err)
	}
	// Another family's child is not a valid target.
	if _, err := svc.Create(ctx, e.spaceA, "", QuestInput{Name: "Q", ChildUID: e.childB}); !errors.Is(err, ErrBadChild) {
		t.Errorf("cross-family child target: %v", err)
	}

	q, err := svc.Create(ctx, e.spaceA, "", QuestInput{Name: "HCF week", Emoji: "🧮", ChildUID: e.childA})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if q.Status != StatusDraft || q.FamilySpaceID != e.spaceA {
		t.Fatalf("quest = %+v", q)
	}

	list, err := svc.BySpace(ctx, e.spaceA)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
	other, err := svc.BySpace(ctx, e.spaceB)
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-space list = %v, %v", other, err)
	}

	// Rename + retarget to all children.
	name, all := "LCM week", ""
	updated, err := svc.Update(ctx, q.UID, UpdateOpts{Name: &name, ChildUID: &all})
	if err != nil || updated.Name != "LCM week" || updated.ChildUID != "" {
		t.Fatalf("update = %+v, %v", updated, err)
	}

	bad := "paused"
	if _, err := svc.Update(ctx, q.UID, UpdateOpts{Status: &bad}); !errors.Is(err, ErrBadStatus) {
		t.Errorf("bad status: %v", err)
	}

	// Delete removes questions and progress too.
	if _, err := svc.AddQuestion(ctx, q.UID, numericQuestion("What is 2 + 3?", "5")); err != nil {
		t.Fatalf("add question: %v", err)
	}
	if err := svc.Delete(ctx, q.UID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Quest(ctx, q.UID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted quest still found: %v", err)
	}
	if n, _ := svc.CountQuestions(ctx, q.UID); n != 0 {
		t.Errorf("questions survived delete: %d", n)
	}
}

func TestPublishGating(t *testing.T) {
	e := newQuestEnv(t)
	svc := New(e.client, nil, nil)
	ctx := context.Background()

	q, err := svc.Create(ctx, e.spaceA, "", QuestInput{Name: "Empty quest"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Publish(ctx, q.UID); !errors.Is(err, ErrNoQuestions) {
		t.Fatalf("publish empty quest: %v", err)
	}
	// Direct status flip is gated the same way.
	active := StatusActive
	if _, err := svc.Update(ctx, q.UID, UpdateOpts{Status: &active}); !errors.Is(err, ErrNoQuestions) {
		t.Fatalf("activate empty quest via update: %v", err)
	}

	if _, err := svc.AddQuestion(ctx, q.UID, numericQuestion("What is 4 + 4?", "8")); err != nil {
		t.Fatalf("add question: %v", err)
	}
	published, err := svc.Publish(ctx, q.UID)
	if err != nil || published.Status != StatusActive {
		t.Fatalf("publish = %+v, %v", published, err)
	}
}

func TestQuestionValidationAndMathcheckWarning(t *testing.T) {
	e := newQuestEnv(t)
	svc := New(e.client, nil, nil)
	ctx := context.Background()
	q, err := svc.Create(ctx, e.spaceA, "", QuestInput{Name: "Quest"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	bad := []QuestionInput{
		{Text: "", Answer: "4", AnswerType: "integer", Format: "numeric"},
		{Text: "Q?", Answer: "", AnswerType: "integer", Format: "numeric"},
		{Text: "Q?", Answer: "4", AnswerType: "integer", Format: "essay"},
		{Text: "Q?", Answer: "4", AnswerType: "roman", Format: "numeric"},
		{Text: "Q?", Answer: "4", AnswerType: "integer", Format: "numeric", Choices: []string{"4", "5"}},
		{Text: "Q?", Answer: "4", AnswerType: "integer", Format: "multiple_choice", Choices: []string{"4"}},
		{Text: "Q?", Answer: "9", AnswerType: "integer", Format: "multiple_choice", Choices: []string{"4", "5"}},
		{Text: "Q?", Answer: "why", AnswerType: "text", Format: "numeric"},
	}
	for i, in := range bad {
		if _, err := svc.AddQuestion(ctx, q.UID, in); !errors.Is(err, ErrBadQuestion) {
			t.Errorf("bad question %d accepted: %v", i, err)
		}
	}

	// A computable question with a wrong answer key saves WITH a warning.
	res, err := svc.AddQuestion(ctx, q.UID, numericQuestion("What is 2 + 2?", "5"))
	if err != nil {
		t.Fatalf("add typo question: %v", err)
	}
	if res.Warning == "" {
		t.Error("expected a mathcheck warning for 2 + 2 = 5")
	}

	// A correct computable question saves clean.
	res, err = svc.AddQuestion(ctx, q.UID, numericQuestion("What is 2 + 2?", "4"))
	if err != nil || res.Warning != "" {
		t.Fatalf("clean question: warning=%q err=%v", res.Warning, err)
	}

	// Fixing the typo via update clears the warning.
	fixed, err := svc.UpdateQuestion(ctx, q.UID, res.Question.UID, numericQuestion("What is 3 + 3?", "6"))
	if err != nil || fixed.Warning != "" || fixed.Question.Text != "What is 3 + 3?" {
		t.Fatalf("update question = %+v, %v", fixed, err)
	}

	// Questions come back in authored order.
	questions, err := svc.Questions(ctx, q.UID)
	if err != nil || len(questions) != 2 {
		t.Fatalf("questions = %d, %v", len(questions), err)
	}
	if questions[0].Text != "What is 2 + 2?" || questions[1].Text != "What is 3 + 3?" {
		t.Errorf("order = %q, %q", questions[0].Text, questions[1].Text)
	}

	// A question UID from another quest 404s instead of leaking.
	q2, _ := svc.Create(ctx, e.spaceA, "", QuestInput{Name: "Other"})
	if _, err := svc.UpdateQuestion(ctx, q2.UID, questions[0].UID, numericQuestion("X?", "1")); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-quest question update: %v", err)
	}
}

func TestGenerateDebitIdempotencyAndValidation(t *testing.T) {
	e := newQuestEnv(t)
	ctx := context.Background()
	if err := e.credits.Grant(ctx, e.spaceA, credits.KindTopup, 10, nil, "test:grant-a"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	svc := New(e.client, e.credits, mockProviderFactory(
		llm.MockResponse{Content: []byte(goodBatchJSON)},
	))

	q, err := svc.Create(ctx, e.spaceA, "", QuestInput{Name: "Gen quest"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Input validation.
	if _, err := svc.Generate(ctx, q.UID, "", 5, "k"); !errors.Is(err, ErrBadBrief) {
		t.Errorf("empty brief: %v", err)
	}
	if _, err := svc.Generate(ctx, q.UID, "b", 0, "k"); !errors.Is(err, ErrBadCount) {
		t.Errorf("zero count: %v", err)
	}
	if _, err := svc.Generate(ctx, q.UID, "b", MaxGenerateCount+1, "k"); !errors.Is(err, ErrBadCount) {
		t.Errorf("huge count: %v", err)
	}
	if _, err := svc.Generate(ctx, q.UID, "b", 5, " "); !errors.Is(err, ErrBadKey) {
		t.Errorf("empty key: %v", err)
	}

	res, err := svc.Generate(ctx, q.UID, "10 easy addition problems, grade 3", 2, "click-1")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(res.Questions) != 2 || res.Replayed {
		t.Fatalf("generate = %d questions, replayed=%v (invalid question should be dropped)", len(res.Questions), res.Replayed)
	}
	for _, qq := range res.Questions {
		if qq.ClientKey != "click-1" {
			t.Errorf("question missing client key: %+v", qq)
		}
	}
	balance, err := e.credits.Balance(ctx, e.spaceA)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if balance != 9 { // ceil(2/5) = 1 debited
		t.Fatalf("balance after generate = %d, want 9", balance)
	}

	// Retry of the same click: same batch back, no new debit, no new rows.
	res2, err := svc.Generate(ctx, q.UID, "10 easy addition problems, grade 3", 2, "click-1")
	if err != nil {
		t.Fatalf("replay generate: %v", err)
	}
	if !res2.Replayed || len(res2.Questions) != 2 {
		t.Fatalf("replay = %+v", res2)
	}
	if balance, _ := e.credits.Balance(ctx, e.spaceA); balance != 9 {
		t.Errorf("replay debited again: balance = %d", balance)
	}
	if n, _ := svc.CountQuestions(ctx, q.UID); n != 2 {
		t.Errorf("replay duplicated questions: %d", n)
	}

	// Generation is refused on published quests: publishing is the
	// approval gate.
	if _, err := svc.Publish(ctx, q.UID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, err := svc.Generate(ctx, q.UID, "more", 2, "click-2"); !errors.Is(err, ErrBadStatus) {
		t.Errorf("generate into active quest: %v", err)
	}
}

func TestGenerateInsufficientCreditsWritesNothing(t *testing.T) {
	e := newQuestEnv(t)
	ctx := context.Background()
	// Space B has no credits at all.
	svc := New(e.client, e.credits, mockProviderFactory(
		llm.MockResponse{Content: []byte(goodBatchJSON)},
	))
	q, err := svc.Create(ctx, e.spaceB, "", QuestInput{Name: "Broke quest"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Generate(ctx, q.UID, "brief", 2, "click-1"); !errors.Is(err, credits.ErrInsufficient) {
		t.Fatalf("generate with empty wallet: %v", err)
	}
	if n, _ := svc.CountQuestions(ctx, q.UID); n != 0 {
		t.Errorf("questions saved despite failed debit: %d", n)
	}
}

func TestGenerateFreeWhenBillingOff(t *testing.T) {
	e := newQuestEnv(t)
	ctx := context.Background()
	svc := New(e.client, nil, mockProviderFactory(
		llm.MockResponse{Content: []byte(goodBatchJSON)},
	))
	q, err := svc.Create(ctx, e.spaceA, "", QuestInput{Name: "Free quest"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	res, err := svc.Generate(ctx, q.UID, "brief", 2, "click-1")
	if err != nil || len(res.Questions) != 2 {
		t.Fatalf("free generate = %+v, %v", res, err)
	}

	// No provider configured → generation unavailable, authoring still fine.
	noAI := New(e.client, nil, nil)
	q2, _ := noAI.Create(ctx, e.spaceA, "", QuestInput{Name: "Manual quest"})
	if _, err := noAI.Generate(ctx, q2.UID, "brief", 2, "k"); !errors.Is(err, ErrNoProvider) {
		t.Errorf("generate without provider: %v", err)
	}
}

func TestPlayableQuestTargetingAndProgress(t *testing.T) {
	e := newQuestEnv(t)
	svc := New(e.client, nil, nil)
	ctx := context.Background()

	q, err := svc.Create(ctx, e.spaceA, "", QuestInput{Name: "Play quest", ChildUID: e.childA})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var uids []string
	for i := 0; i < 3; i++ {
		res, err := svc.AddQuestion(ctx, q.UID, numericQuestion(fmt.Sprintf("What is %d + 1?", i), fmt.Sprintf("%d", i+1)))
		if err != nil {
			t.Fatalf("add question %d: %v", i, err)
		}
		uids = append(uids, res.Question.UID)
	}

	// Draft quests are not playable.
	if _, err := svc.PlayableQuest(ctx, e.childA, q.UID); !errors.Is(err, game.ErrQuestUnavailable) {
		t.Errorf("draft playable: %v", err)
	}
	if _, err := svc.Publish(ctx, q.UID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Cross-family child: unavailable, not leaked.
	if _, err := svc.PlayableQuest(ctx, e.childB, q.UID); !errors.Is(err, game.ErrQuestUnavailable) {
		t.Errorf("cross-family playable: %v", err)
	}
	// Same family but not the targeted child.
	otherKid, err := e.family.AddChild(ctx, e.spaceA, "Sibling", 4, "")
	if err != nil {
		t.Fatalf("sibling: %v", err)
	}
	if _, err := svc.PlayableQuest(ctx, otherKid.UID, q.UID); !errors.Is(err, game.ErrQuestUnavailable) {
		t.Errorf("mis-targeted playable: %v", err)
	}

	play, err := svc.PlayableQuest(ctx, e.childA, q.UID)
	if err != nil {
		t.Fatalf("playable: %v", err)
	}
	if len(play.Questions) != 3 || play.Questions[0].UID != uids[0] {
		t.Fatalf("play = %+v", play)
	}

	// Wrong answer: progress recorded, nothing consumed.
	remaining, err := svc.RecordAnswer(ctx, q.UID, e.childA, uids[0], false)
	if err != nil || remaining != 3 {
		t.Fatalf("wrong answer remaining = %d, %v", remaining, err)
	}
	// Correct answers consume; correct is sticky across a later wrong one.
	if remaining, _ = svc.RecordAnswer(ctx, q.UID, e.childA, uids[0], true); remaining != 2 {
		t.Fatalf("remaining after first correct = %d", remaining)
	}
	if remaining, _ = svc.RecordAnswer(ctx, q.UID, e.childA, uids[0], false); remaining != 2 {
		t.Fatalf("correct not sticky: remaining = %d", remaining)
	}

	// The next play serves only not-yet-correct questions, in order.
	play, err = svc.PlayableQuest(ctx, e.childA, q.UID)
	if err != nil || len(play.Questions) != 2 || play.Questions[0].UID != uids[1] {
		t.Fatalf("second play = %+v, %v", play, err)
	}

	// Finishing every question: quest is done.
	_, _ = svc.RecordAnswer(ctx, q.UID, e.childA, uids[1], true)
	if remaining, _ = svc.RecordAnswer(ctx, q.UID, e.childA, uids[2], true); remaining != 0 {
		t.Fatalf("remaining after all correct = %d", remaining)
	}
	if _, err := svc.PlayableQuest(ctx, e.childA, q.UID); !errors.Is(err, game.ErrQuestDone) {
		t.Errorf("finished quest playable: %v", err)
	}

	// Progress is per child: the sibling has everything left on an
	// all-children quest.
	all := ""
	if _, err := svc.Update(ctx, q.UID, UpdateOpts{ChildUID: &all}); err != nil {
		t.Fatalf("retarget: %v", err)
	}
	sibPlay, err := svc.PlayableQuest(ctx, otherKid.UID, q.UID)
	if err != nil || len(sibPlay.Questions) != 3 {
		t.Fatalf("sibling play = %+v, %v", sibPlay, err)
	}
}

func TestActiveQuestsListing(t *testing.T) {
	e := newQuestEnv(t)
	svc := New(e.client, nil, nil)
	ctx := context.Background()

	mk := func(space, name, childUID string, publish bool) *ent.Quest {
		q, err := svc.Create(ctx, space, "", QuestInput{Name: name, ChildUID: childUID})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := svc.AddQuestion(ctx, q.UID, numericQuestion("What is 1 + 1?", "2")); err != nil {
			t.Fatalf("question %s: %v", name, err)
		}
		if publish {
			if _, err := svc.Publish(ctx, q.UID); err != nil {
				t.Fatalf("publish %s: %v", name, err)
			}
		}
		return q
	}

	forA := mk(e.spaceA, "For A", e.childA, true)
	mk(e.spaceA, "For everyone", "", true)
	mk(e.spaceA, "Draft", "", false)
	mk(e.spaceB, "Other family", "", true)
	otherKid, _ := e.family.AddChild(ctx, e.spaceA, "Sibling", 4, "")
	mk(e.spaceA, "For sibling", otherKid.UID, true)

	items, err := svc.ActiveQuests(ctx, e.childA)
	if err != nil {
		t.Fatalf("active quests: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v, want 2 (targeted + all-children)", items)
	}
	if items[0].Name != "For A" || items[1].Name != "For everyone" {
		t.Errorf("items = %+v", items)
	}
	if items[0].Total != 1 || items[0].Correct != 0 || items[0].Done {
		t.Errorf("progress = %+v", items[0])
	}

	// Completion shows up as done.
	questions, _ := svc.Questions(ctx, forA.UID)
	if _, err := svc.RecordAnswer(ctx, forA.UID, e.childA, questions[0].UID, true); err != nil {
		t.Fatalf("record: %v", err)
	}
	items, _ = svc.ActiveQuests(ctx, e.childA)
	if !items[0].Done || items[0].Correct != 1 {
		t.Errorf("done item = %+v", items[0])
	}

	// Unknown child: empty, not an error (and never other families' quests).
	items, err = svc.ActiveQuests(ctx, "no-such-child")
	if err != nil || len(items) != 0 {
		t.Errorf("unknown child = %+v, %v", items, err)
	}
}

func TestAuthzQuestChecks(t *testing.T) {
	e := newQuestEnv(t)
	svc := New(e.client, nil, nil)
	ctx := context.Background()

	checker := authz.NewChecker(e.family)
	checker.SetQuests(svc)

	q, err := svc.Create(ctx, e.spaceA, "", QuestInput{Name: "Authz quest", ChildUID: e.childA})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	parentA := authz.Principal{Kind: authz.KindParent, AccountID: e.acctA}
	parentB := authz.Principal{Kind: authz.KindParent, AccountID: e.acctB}
	childA := authz.Principal{Kind: authz.KindChild, ChildProfileID: e.childA, FamilySpaceID: e.spaceA}
	childB := authz.Principal{Kind: authz.KindChild, ChildProfileID: e.childB, FamilySpaceID: e.spaceB}

	if err := checker.CanManageQuest(ctx, parentA, q.UID); err != nil {
		t.Errorf("owner manage: %v", err)
	}
	if err := checker.CanManageQuest(ctx, parentB, q.UID); !errors.Is(err, authz.ErrDenied) {
		t.Errorf("cross-family manage: %v", err)
	}
	if err := checker.CanManageQuest(ctx, childA, q.UID); !errors.Is(err, authz.ErrDenied) {
		t.Errorf("child manage: %v", err)
	}
	if err := checker.CanManageQuest(ctx, parentA, "no-such-quest"); !errors.Is(err, authz.ErrDenied) {
		t.Errorf("missing quest manage: %v", err)
	}

	// Draft quests are not playable even for the targeted child.
	if err := checker.CanPlayQuest(ctx, childA, q.UID); !errors.Is(err, authz.ErrDenied) {
		t.Errorf("draft play: %v", err)
	}
	if _, err := svc.AddQuestion(ctx, q.UID, numericQuestion("What is 1 + 1?", "2")); err != nil {
		t.Fatalf("question: %v", err)
	}
	if _, err := svc.Publish(ctx, q.UID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := checker.CanPlayQuest(ctx, childA, q.UID); err != nil {
		t.Errorf("targeted child play: %v", err)
	}
	if err := checker.CanPlayQuest(ctx, childB, q.UID); !errors.Is(err, authz.ErrDenied) {
		t.Errorf("cross-family play: %v", err)
	}
	if err := checker.CanPlayQuest(ctx, parentA, q.UID); !errors.Is(err, authz.ErrDenied) {
		t.Errorf("parent play: %v", err)
	}

	// Same family, different target child.
	sibling, err := e.family.AddChild(ctx, e.spaceA, "Sibling", 4, "")
	if err != nil {
		t.Fatalf("sibling: %v", err)
	}
	sibPrincipal := authz.Principal{Kind: authz.KindChild, ChildProfileID: sibling.UID, FamilySpaceID: e.spaceA}
	if err := checker.CanPlayQuest(ctx, sibPrincipal, q.UID); !errors.Is(err, authz.ErrDenied) {
		t.Errorf("mis-targeted play: %v", err)
	}
	// Retargeting to all children opens it up.
	all := ""
	if _, err := svc.Update(ctx, q.UID, UpdateOpts{ChildUID: &all}); err != nil {
		t.Fatalf("retarget: %v", err)
	}
	if err := checker.CanPlayQuest(ctx, sibPrincipal, q.UID); err != nil {
		t.Errorf("all-children play: %v", err)
	}

	// A checker without a quest directory fails closed.
	bare := authz.NewChecker(e.family)
	if err := bare.CanManageQuest(ctx, parentA, q.UID); !errors.Is(err, authz.ErrDenied) {
		t.Errorf("no directory manage: %v", err)
	}
	if err := bare.CanPlayQuest(ctx, childA, q.UID); !errors.Is(err, authz.ErrDenied) {
		t.Errorf("no directory play: %v", err)
	}
}
