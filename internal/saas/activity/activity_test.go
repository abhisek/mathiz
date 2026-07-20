package activity

import (
	"context"
	"errors"
	"testing"

	"github.com/abhisek/mathiz/internal/store"
)

// fakeQuests is a canned QuestMeta lookup.
type fakeQuests struct {
	name, emoji, createdBy string
	err                    error
	calls                  []string
}

func (f *fakeQuests) QuestMeta(_ context.Context, questUID string) (string, string, string, error) {
	f.calls = append(f.calls, questUID)
	if f.err != nil {
		return "", "", "", f.err
	}
	return f.name, f.emoji, f.createdBy, nil
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// seedStream writes a realistic child event stream and returns the child UID.
// Sequence order (oldest → newest):
//
//	sess-1 start (skill pv-hundreds) → 2 answers → hint → lesson →
//	mastery learning→mastered → gem → sess-1 end →
//	mastery learning→learning (noise, filtered) →
//	sess-2 start (untagged quest "quest:q-123") → answer → sess-2 end
func seedStream(t *testing.T, st *store.Store, childUID string) {
	t.Helper()
	ctx := context.Background()
	repo := st.EventRepoFor(childUID)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	must(repo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID: "sess-1", Action: "start",
		PlanSummary: []store.PlanSlotSummaryData{{SkillID: "pv-hundreds", Tier: "learn", Category: "frontier"}},
	}))
	must(repo.AppendAnswerEvent(ctx, store.AnswerEventData{
		SessionID: "sess-1", SkillID: "pv-hundreds", Tier: "learn", Category: "frontier",
		QuestionText: "What is the value of 3 in 345?", CorrectAnswer: "300",
		LearnerAnswer: "300", Correct: true, TimeMs: 4200, AnswerFormat: "integer",
	}))
	must(repo.AppendAnswerEvent(ctx, store.AnswerEventData{
		SessionID: "sess-1", SkillID: "pv-hundreds", Tier: "learn", Category: "frontier",
		QuestionText: "What is the value of 4 in 345?", CorrectAnswer: "40",
		LearnerAnswer: "4", Correct: false, TimeMs: 8100, AnswerFormat: "integer",
	}))
	must(repo.AppendHintEvent(ctx, store.HintEventData{
		SessionID: "sess-1", SkillID: "pv-hundreds",
		QuestionText: "What is the value of 4 in 345?", HintText: "Which place is the 4 in?",
	}))
	must(repo.AppendLessonEvent(ctx, store.LessonEventData{
		SessionID: "sess-1", SkillID: "pv-hundreds", LessonTitle: "Tens place tip",
		Explanation: "The middle digit counts tens.",
	}))
	must(repo.AppendMasteryEvent(ctx, store.MasteryEventData{
		SkillID: "pv-hundreds", FromState: "learning", ToState: "mastered",
		Trigger: "fluency", FluencyScore: 0.92, SessionID: "sess-1",
	}))
	must(repo.AppendGemEvent(ctx, store.GemEventData{
		GemType: "mastery", Rarity: "rare", SessionID: "sess-1", Reason: "mastered pv-hundreds",
	}))
	must(repo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID: "sess-1", Action: "end", QuestionsServed: 2, CorrectAnswers: 1, DurationSecs: 312,
	}))
	// A transition parents don't care about: must be filtered out.
	must(repo.AppendMasteryEvent(ctx, store.MasteryEventData{
		SkillID: "compare-1000", FromState: "new", ToState: "learning",
		Trigger: "attempt", FluencyScore: 0.1, SessionID: "sess-1",
	}))
	// Untagged quest session: the synthetic quest plan ID.
	must(repo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID: "sess-2", Action: "start",
		PlanSummary: []store.PlanSlotSummaryData{{SkillID: "quest:q-123", Tier: "learn", Category: "review"}},
	}))
	must(repo.AppendAnswerEvent(ctx, store.AnswerEventData{
		SessionID: "sess-2", SkillID: "quest:q-123", Tier: "learn", Category: "review",
		QuestionText: "What is 12 + 8?", CorrectAnswer: "20",
		LearnerAnswer: "20", Correct: true, TimeMs: 3000, AnswerFormat: "integer",
	}))
	must(repo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID: "sess-2", Action: "end", QuestionsServed: 1, CorrectAnswers: 1, DurationSecs: 60,
	}))
}

func TestTimelineMergeAndAttribution(t *testing.T) {
	st := openTestStore(t)
	const child = "child-1"
	seedStream(t, st, child)

	quests := &fakeQuests{name: "Space Quest", emoji: "⭐", createdBy: "acct-1"}
	r := NewReader(st, quests, func(_ context.Context, accountID string) (string, error) {
		if accountID != "acct-1" {
			t.Errorf("member lookup for %q, want acct-1", accountID)
		}
		return "Abhisek", nil
	})

	page, err := r.Timeline(context.Background(), child, TimelineQuery{})
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	// Expected newest-first: sess-2 expedition, sess-1 expedition,
	// mastered transition, lesson. (learning→learning noise filtered.)
	kinds := make([]string, len(page.Items))
	for i, it := range page.Items {
		kinds[i] = it.Kind
	}
	wantKinds := []string{KindExpedition, KindExpedition, KindMastery, KindLesson}
	if len(kinds) != len(wantKinds) {
		t.Fatalf("items = %v, want kinds %v", kinds, wantKinds)
	}
	for i := range wantKinds {
		if kinds[i] != wantKinds[i] {
			t.Fatalf("item[%d] kind = %s, want %s (all: %v)", i, kinds[i], wantKinds[i], kinds)
		}
	}
	for i := 1; i < len(page.Items); i++ {
		if page.Items[i].Seq >= page.Items[i-1].Seq {
			t.Errorf("items not newest-first at %d: %d then %d", i, page.Items[i-1].Seq, page.Items[i].Seq)
		}
	}
	if page.NextBefore != 0 {
		t.Errorf("NextBefore = %d, want 0 on a short page", page.NextBefore)
	}

	// Quest expedition: attribution resolved, no phantom skills.
	quest := page.Items[0].Expedition
	if quest.SessionID != "sess-2" || quest.Questions != 1 || quest.Correct != 1 {
		t.Errorf("quest expedition = %+v", quest)
	}
	if quest.Quest == nil || quest.Quest.ID != "q-123" || quest.Quest.Name != "Space Quest" ||
		quest.Quest.Emoji != "⭐" || quest.Quest.CreatedBy != "Abhisek" {
		t.Errorf("quest attribution = %+v", quest.Quest)
	}
	if len(quest.Skills) != 0 {
		t.Errorf("quest expedition skills = %+v, want none (synthetic ID is attribution)", quest.Skills)
	}
	if quest.Category != "review" {
		t.Errorf("quest expedition category = %q, want review (first plan slot)", quest.Category)
	}

	// Normal expedition: skills resolved via the graph, gems joined.
	dig := page.Items[1].Expedition
	if dig.SessionID != "sess-1" || dig.Questions != 2 || dig.Correct != 1 ||
		dig.DurationSecs != 312 || dig.Gems != 1 {
		t.Errorf("dig expedition = %+v", dig)
	}
	if len(dig.Skills) != 1 || dig.Skills[0].ID != "pv-hundreds" ||
		dig.Skills[0].Name != "Place Value to 1,000" {
		t.Errorf("dig skills = %+v", dig.Skills)
	}
	if dig.Quest != nil {
		t.Errorf("dig quest = %+v, want nil", dig.Quest)
	}
	if dig.Category != "frontier" {
		t.Errorf("dig category = %q, want frontier (first plan slot)", dig.Category)
	}

	// Mastery + lesson payloads.
	m := page.Items[2].Mastery
	if m.SkillID != "pv-hundreds" || m.SkillName != "Place Value to 1,000" ||
		m.FromState != "learning" || m.ToState != "mastered" {
		t.Errorf("mastery item = %+v", m)
	}
	l := page.Items[3].Lesson
	if l.SkillID != "pv-hundreds" || l.SkillName != "Place Value to 1,000" || l.Title != "Tens place tip" {
		t.Errorf("lesson item = %+v", l)
	}
}

func TestTimelinePaginationWalk(t *testing.T) {
	st := openTestStore(t)
	const child = "child-1"
	seedStream(t, st, child)
	r := NewReader(st, nil, nil)
	ctx := context.Background()

	// Walk the whole timeline in pages of 2.
	var all []TimelineItem
	before := int64(0)
	for range 10 {
		page, err := r.Timeline(ctx, child, TimelineQuery{Before: before, Limit: 2})
		if err != nil {
			t.Fatalf("page before=%d: %v", before, err)
		}
		if len(page.Items) > 2 {
			t.Fatalf("page size = %d, want <= 2", len(page.Items))
		}
		all = append(all, page.Items...)
		if page.NextBefore == 0 {
			break
		}
		if page.NextBefore != page.Items[len(page.Items)-1].Seq {
			t.Errorf("NextBefore = %d, want lowest seq %d", page.NextBefore, page.Items[len(page.Items)-1].Seq)
		}
		before = page.NextBefore
	}
	if len(all) != 4 {
		t.Fatalf("walked %d items, want 4", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].Seq >= all[i-1].Seq {
			t.Errorf("walk not strictly descending at %d", i)
		}
	}

	// Without a quests lookup, the quest ref degrades to the bare ID.
	if q := all[0].Expedition.Quest; q == nil || q.ID != "q-123" || q.Name != "" || q.CreatedBy != "" {
		t.Errorf("degraded quest ref = %+v", q)
	}
}

// seedQuestSession writes one finished session attributed to a quest via the
// durable start-event fields (QuestUID/QuestName), planning skillID — a real
// skill for tagged quests. Returns nothing; sequence order follows call order.
func seedQuestSession(t *testing.T, st *store.Store, childUID, sessionID, questUID, questName, skillID string) {
	t.Helper()
	ctx := context.Background()
	repo := st.EventRepoFor(childUID)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed quest session: %v", err)
		}
	}
	must(repo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID: sessionID, Action: "start",
		PlanSummary: []store.PlanSlotSummaryData{{SkillID: skillID, Tier: "learn", Category: "frontier"}},
		QuestUID:    questUID, QuestName: questName,
	}))
	must(repo.AppendAnswerEvent(ctx, store.AnswerEventData{
		SessionID: sessionID, SkillID: skillID, Tier: "learn", Category: "frontier",
		QuestionText: "What is 6 + 6?", CorrectAnswer: "12",
		LearnerAnswer: "12", Correct: true, TimeMs: 2000, AnswerFormat: "integer",
	}))
	must(repo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID: sessionID, Action: "end", QuestionsServed: 1, CorrectAnswers: 1, DurationSecs: 45,
	}))
}

// TestTaggedQuestSessionAttribution: a tagged quest session plans the REAL
// skill, so attribution must come from the start event's QuestUID/QuestName.
// The event's as-of-play name wins over the live lookup (rename-proof); the
// lookup only enriches emoji/createdBy.
func TestTaggedQuestSessionAttribution(t *testing.T) {
	st := openTestStore(t)
	const child = "child-1"
	seedQuestSession(t, st, child, "sess-q", "q-tag", "HCF Week", "pv-hundreds")

	quests := &fakeQuests{name: "Renamed Since", emoji: "🧭", createdBy: "acct-1"}
	r := NewReader(st, quests, func(_ context.Context, _ string) (string, error) {
		return "Abhisek", nil
	})
	page, err := r.Timeline(context.Background(), child, TimelineQuery{Kinds: []string{KindExpedition}})
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(page.Items))
	}
	exp := page.Items[0].Expedition
	q := exp.Quest
	if q == nil || q.ID != "q-tag" {
		t.Fatalf("quest ref = %+v, want ID q-tag", q)
	}
	if q.Name != "HCF Week" {
		t.Errorf("quest name = %q, want the as-of-play event name HCF Week", q.Name)
	}
	if q.Emoji != "🧭" || q.CreatedBy != "Abhisek" {
		t.Errorf("enrichment = (%q, %q), want (🧭, Abhisek)", q.Emoji, q.CreatedBy)
	}
	// The real tagged skill still shows as a skill.
	if len(exp.Skills) != 1 || exp.Skills[0].ID != "pv-hundreds" {
		t.Errorf("skills = %+v, want pv-hundreds", exp.Skills)
	}
}

// TestQuestAttributionSurvivesDeletion: no Quests lookup at all, and an
// erroring lookup, both keep the name that came from the event.
func TestQuestAttributionSurvivesDeletion(t *testing.T) {
	st := openTestStore(t)
	const child = "child-1"
	seedQuestSession(t, st, child, "sess-q", "q-gone", "Deleted Quest", "pv-hundreds")

	for name, r := range map[string]*Reader{
		"nil lookup":      NewReader(st, nil, nil),
		"erroring lookup": NewReader(st, &fakeQuests{err: errors.New("quest deleted")}, nil),
	} {
		page, err := r.Timeline(context.Background(), child, TimelineQuery{Kinds: []string{KindExpedition}})
		if err != nil {
			t.Fatalf("%s: timeline: %v", name, err)
		}
		q := page.Items[0].Expedition.Quest
		if q == nil || q.ID != "q-gone" || q.Name != "Deleted Quest" {
			t.Errorf("%s: quest ref = %+v, want ID q-gone name from event", name, q)
		}
	}
}

// TestTimelineQuestFilter: quest=<uid> returns only matching expeditions and
// pages past stretches of non-matching sessions instead of returning an
// empty page with no cursor.
func TestTimelineQuestFilter(t *testing.T) {
	st := openTestStore(t)
	const child = "child-1"
	ctx := context.Background()
	repo := st.EventRepoFor(child)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// Oldest → newest: q-A (event fields), 4 normal digs, q-A again via the
	// LEGACY synthetic plan prefix, one session of a different quest.
	seedQuestSession(t, st, child, "sess-q1", "q-A", "Quest A", "pv-hundreds")
	for i := 0; i < 4; i++ {
		sid := "sess-dig-" + string(rune('a'+i))
		must(repo.AppendSessionEvent(ctx, store.SessionEventData{
			SessionID: sid, Action: "start",
			PlanSummary: []store.PlanSlotSummaryData{{SkillID: "pv-hundreds", Tier: "learn", Category: "frontier"}},
		}))
		must(repo.AppendSessionEvent(ctx, store.SessionEventData{
			SessionID: sid, Action: "end", QuestionsServed: 1, CorrectAnswers: 1, DurationSecs: 30,
		}))
	}
	must(repo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID: "sess-q2", Action: "start",
		PlanSummary: []store.PlanSlotSummaryData{{SkillID: "quest:q-A", Tier: "learn", Category: "review"}},
	}))
	must(repo.AppendSessionEvent(ctx, store.SessionEventData{
		SessionID: "sess-q2", Action: "end", QuestionsServed: 1, CorrectAnswers: 0, DurationSecs: 20,
	}))
	seedQuestSession(t, st, child, "sess-other", "q-B", "Quest B", "pv-hundreds")

	r := NewReader(st, nil, nil)

	// Limit 2: page 1 must dig past the newest non-matching session AND the
	// 4-dig stretch to find both q-A sessions (durable + legacy attribution).
	page, err := r.Timeline(ctx, child, TimelineQuery{QuestUID: "q-A", Limit: 2})
	if err != nil {
		t.Fatalf("quest filter: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("page 1 items = %d, want 2", len(page.Items))
	}
	if got := page.Items[0].Expedition; got.SessionID != "sess-q2" || got.Quest == nil || got.Quest.ID != "q-A" {
		t.Errorf("page 1 item 0 = %+v quest %+v", got, got.Quest)
	}
	if got := page.Items[1].Expedition; got.SessionID != "sess-q1" || got.Quest == nil ||
		got.Quest.ID != "q-A" || got.Quest.Name != "Quest A" {
		t.Errorf("page 1 item 1 = %+v quest %+v", got, got.Quest)
	}
	// Full page → cursor present; the next page walks to exhaustion, empty.
	if page.NextBefore == 0 {
		t.Fatal("full filtered page must carry a cursor")
	}
	page2, err := r.Timeline(ctx, child, TimelineQuery{QuestUID: "q-A", Limit: 2, Before: page.NextBefore})
	if err != nil {
		t.Fatalf("quest filter page 2: %v", err)
	}
	if len(page2.Items) != 0 || page2.NextBefore != 0 {
		t.Errorf("page 2 = %+v, want empty end of stream", page2.Items)
	}

	// The kinds param is ignored under a quest filter (documented behavior):
	// even a bogus kind doesn't 400 and only expeditions come back.
	page, err = r.Timeline(ctx, child, TimelineQuery{QuestUID: "q-B", Kinds: []string{"mastery"}})
	if err != nil {
		t.Fatalf("quest filter with kinds: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Kind != KindExpedition ||
		page.Items[0].Expedition.SessionID != "sess-other" {
		t.Errorf("q-B filter = %+v", page.Items)
	}

	// Unknown quest → empty, not an error.
	page, err = r.Timeline(ctx, child, TimelineQuery{QuestUID: "q-nope"})
	if err != nil || len(page.Items) != 0 {
		t.Errorf("unknown quest filter = %v items %d", err, len(page.Items))
	}

	// Owner isolation holds under the filter too.
	page, err = r.Timeline(ctx, "child-2", TimelineQuery{QuestUID: "q-A"})
	if err != nil || len(page.Items) != 0 {
		t.Errorf("cross-child quest filter = %v items %d", err, len(page.Items))
	}
}

func TestTimelineKindFilter(t *testing.T) {
	st := openTestStore(t)
	const child = "child-1"
	seedStream(t, st, child)
	r := NewReader(st, nil, nil)
	ctx := context.Background()

	page, err := r.Timeline(ctx, child, TimelineQuery{Kinds: []string{KindMastery}})
	if err != nil {
		t.Fatalf("mastery filter: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Kind != KindMastery {
		t.Fatalf("mastery-only page = %+v", page.Items)
	}

	page, err = r.Timeline(ctx, child, TimelineQuery{Kinds: []string{KindExpedition, KindLesson}})
	if err != nil {
		t.Fatalf("expedition+lesson filter: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expedition+lesson page = %d items, want 3", len(page.Items))
	}
	for _, it := range page.Items {
		if it.Kind == KindMastery {
			t.Errorf("mastery item leaked through filter")
		}
	}

	if _, err := r.Timeline(ctx, child, TimelineQuery{Kinds: []string{"gems"}}); !errors.Is(err, ErrBadKind) {
		t.Errorf("unknown kind err = %v, want ErrBadKind", err)
	}
}

func TestTimelineIsOwnerScoped(t *testing.T) {
	st := openTestStore(t)
	seedStream(t, st, "child-1")
	r := NewReader(st, nil, nil)

	page, err := r.Timeline(context.Background(), "child-2", TimelineQuery{})
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if len(page.Items) != 0 {
		t.Errorf("child-2 sees %d of child-1's items, want 0", len(page.Items))
	}
}

func TestSessionDetail(t *testing.T) {
	st := openTestStore(t)
	const child = "child-1"
	seedStream(t, st, child)
	r := NewReader(st, nil, nil)
	ctx := context.Background()

	detail, err := r.SessionDetail(ctx, child, "sess-1")
	if err != nil {
		t.Fatalf("session detail: %v", err)
	}
	if detail.HintCount != 1 {
		t.Errorf("hint count = %d, want 1", detail.HintCount)
	}
	if len(detail.Answers) != 2 {
		t.Fatalf("answers = %d, want 2", len(detail.Answers))
	}
	// Oldest first (question order).
	first, second := detail.Answers[0], detail.Answers[1]
	if first.Seq >= second.Seq {
		t.Errorf("answers not oldest-first: %d then %d", first.Seq, second.Seq)
	}
	if !first.Correct || first.LearnerAnswer != "300" || first.TimeMs != 4200 ||
		first.SkillName != "Place Value to 1,000" {
		t.Errorf("first answer = %+v", first)
	}
	if second.Correct || second.LearnerAnswer != "4" || second.CorrectAnswer != "40" {
		t.Errorf("second answer = %+v", second)
	}

	// Unknown session → typed not-found.
	if _, err := r.SessionDetail(ctx, child, "no-such-session"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("unknown session err = %v, want ErrSessionNotFound", err)
	}
	// Another child asking for this session sees nothing (owner scoping).
	if _, err := r.SessionDetail(ctx, "child-2", "sess-1"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("cross-child session err = %v, want ErrSessionNotFound", err)
	}
}
