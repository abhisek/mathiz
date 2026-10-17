package session

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
)

// profileTimeout bounds the async learner-profile refresh. The goroutine
// deliberately runs on context.Background(): the screen or expedition that
// triggered the save is closing, and the refresh must outlive it.
const profileTimeout = 60 * time.Second

// snapshotKeep is how many recent snapshots survive pruning after each save.
const snapshotKeep = 10

// SaveSnapshotWithProfile is the single end-of-session persistence path
// shared by the terminal session screen and the game manager. It carries the
// previous learner profile over from the latest snapshot, saves snapData,
// prunes old snapshots, and spawns the async learner-profile compression
// goroutine (which re-loads the latest snapshot and re-saves it with the
// fresh profile).
//
// snapData carries the caller-built Mastery/SpacedRep/Gems snapshot data
// (plus, for the TUI's legacy fallback, TierProgress/MasteredSet). Pass a nil
// compressor to skip the profile refresh. A non-nil error means the snapshot
// save failed and nothing else happened.
//
// ownerID is the learner this save belongs to (child UID in serve mode,
// store.LocalOwner for the CLI). It is carried purely so the async refresh's
// logs are attributable — the repos are already owner-scoped by the caller.
func SaveSnapshotWithProfile(ctx context.Context, ownerID string, snapRepo store.SnapshotRepo, compressor *lessons.Compressor, state *SessionState, snapData store.SnapshotData) error {
	// One Latest fetch serves both the carried-over profile and the
	// PreviousProfile compression input below.
	var prevProfile *store.LearnerProfileData
	prev, err := snapRepo.Latest(ctx)
	switch {
	case err != nil:
		// Without a fallback the profile would simply not be carried into the
		// snapshot we're about to write — silently dropped until something
		// regenerates it. The versioned event stream holds every profile the
		// child has had, so recover the newest from there.
		slog.Error("session: load previous snapshot for profile carry-over",
			"owner_id", ownerID, "err", err)
		prevProfile = recoverProfile(ctx, ownerID, state.EventRepo)
	case prev != nil:
		prevProfile = prev.Data.LearnerProfile
	}
	snapData.LearnerProfile = prevProfile

	if err := snapRepo.Save(ctx, &store.Snapshot{Timestamp: time.Now(), Data: snapData}); err != nil {
		return err
	}
	_ = snapRepo.Prune(ctx, snapshotKeep)

	// No profile refresh without evidence. A session that asked nothing has
	// only untouched plan slots to summarise, and a model asked for strengths
	// and weaknesses anyway will invent them. The guard lives here rather than
	// at the call sites so no future caller can miss it.
	if compressor == nil || state.TotalQuestions == 0 {
		return nil
	}

	// Everything read from live session state is copied here, before the
	// goroutine: the mastery snapshot aliases live slices (SpeedScores), and
	// the session's error map is guarded by ErrorMu.
	input := lessons.ProfileInput{
		PerSkillResults: make(map[string]lessons.SkillResultSummary),
		MasteryData:     make(map[string]lessons.MasteryDataSummary),
		ErrorHistory:    make(map[string][]string),
	}
	for id, r := range state.PerSkillResults {
		// Plan slots are pre-seeded for the whole session, so a skill the
		// child never reached is present with zero attempts. That is absence
		// of evidence, not a failure — leave it out rather than reporting it
		// as 0% correct.
		if r.Attempted == 0 || !curriculumSkill(id) {
			continue
		}
		input.PerSkillResults[id] = lessons.SkillResultSummary{
			Name: r.SkillName, Attempted: r.Attempted, Correct: r.Correct,
		}
	}
	if snapData.Mastery != nil {
		for id, skm := range snapData.Mastery.Skills {
			if !curriculumSkill(id) {
				continue
			}
			sum := skillSummary(id)
			sum.State, sum.Tier, sum.FluencyScore = skm.State, skm.CurrentTier, snapshotFluency(skm)
			input.MasteryData[id] = sum
		}
	}
	state.ErrorMu.Lock()
	for id, errs := range state.RecentErrors {
		if !curriculumSkill(id) {
			continue
		}
		input.ErrorHistory[id] = append([]string(nil), errs...)
	}
	state.ErrorMu.Unlock()
	if prevProfile != nil {
		input.PreviousProfile = &lessons.LearnerProfile{
			Summary: prevProfile.Summary, Strengths: prevProfile.Strengths,
			Weaknesses: prevProfile.Weaknesses, Patterns: prevProfile.Patterns,
		}
	}
	var prevMastery *store.MasterySnapshotData
	if prev != nil {
		prevMastery = prev.Data.Mastery
	}
	masteryChanged := masteryStandingChanged(prevMastery, snapData.Mastery)

	eventRepo := state.EventRepo
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), profileTimeout)
		defer cancel()
		if !groundProfileInput(bg, ownerID, eventRepo, &input, prevProfile, masteryChanged) {
			return
		}
		refreshProfile(bg, ownerID, snapRepo, eventRepo, compressor, input, prevProfile)
	}()
	return nil
}

// Profile refresh policy (FOU-241). The profile describes the whole child,
// and the game ends a session every five questions on one skill, so it is
// not rewritten after every session.
const (
	// profileMinAttempts is the lifetime evidence below which no profile is
	// written: about three expeditions. With less, a model asked for
	// strengths and weaknesses asserts them from a handful of answers.
	profileMinAttempts = 15
	// profileEverySessions is how many completed sessions since the last
	// profile make it due again when nothing else changed.
	profileEverySessions = 3
	// recentSessionLimit bounds the session list in the prompt. It also
	// bounds the sessions-since-profile count, which only needs to reach
	// profileEverySessions.
	recentSessionLimit = 10
	// errorPatternWindow and errorPatternLimit bound the diagnosed error
	// patterns: old errors describe a child who has since moved on.
	errorPatternWindow = 60 * 24 * time.Hour
	errorPatternLimit  = 15
)

// refreshFacts is what profileRefreshDue decides on.
type refreshFacts struct {
	LifetimeAttempts     int
	HasProfile           bool
	MasteryChanged       bool
	SessionsSinceProfile int
}

// profileRefreshDue decides whether to regenerate the profile, and says why
// for the log.
func profileRefreshDue(f refreshFacts) (bool, string) {
	switch {
	case f.LifetimeAttempts < profileMinAttempts:
		return false, "too little evidence"
	case !f.HasProfile:
		return true, "first profile"
	case f.MasteryChanged:
		return true, "mastery changed"
	case f.SessionsSinceProfile >= profileEverySessions:
		return true, "sessions since profile"
	default:
		return false, "not due"
	}
}

// groundProfileInput loads the child's history into input and decides
// whether a refresh is due. It runs inside the async refresh, so the reads
// never slow the end of a session. A failed read skips the refresh: a
// profile made from this session alone is the defect this exists to stop.
func groundProfileInput(ctx context.Context, ownerID string, eventRepo store.EventRepo, input *lessons.ProfileInput, prevProfile *store.LearnerProfileData, masteryChanged bool) bool {
	if eventRepo == nil {
		slog.Error("session: learner profile skipped: no event history", "owner_id", ownerID)
		return false
	}
	fail := func(what string, err error) bool {
		slog.Error("session: learner profile skipped: load "+what, "owner_id", ownerID, "err", err)
		return false
	}

	tallies, err := eventRepo.AnswerTallies(ctx)
	if err != nil {
		return fail("answer totals", err)
	}
	lifetime := 0
	for _, t := range tallies {
		if !curriculumSkill(t.SkillID) {
			continue
		}
		sum, ok := input.MasteryData[t.SkillID]
		if !ok {
			sum = skillSummary(t.SkillID)
		}
		sum.Attempts, sum.Correct = t.Attempts, t.Correct
		input.MasteryData[t.SkillID] = sum
		lifetime += t.Attempts
	}

	summaries, err := eventRepo.QuerySessionSummaries(ctx, store.QueryOpts{Limit: recentSessionLimit})
	if err != nil {
		return fail("recent sessions", err)
	}
	input.RecentSessions = nil
	for _, s := range summaries {
		if sum, ok := sessionSummary(s); ok {
			input.RecentSessions = append(input.RecentSessions, sum)
		}
	}

	facts := refreshFacts{
		LifetimeAttempts:     lifetime,
		HasProfile:           prevProfile != nil,
		MasteryChanged:       masteryChanged,
		SessionsSinceProfile: sessionsSince(input.RecentSessions, prevProfile),
	}
	due, reason := profileRefreshDue(facts)
	if !due {
		slog.Info("session: learner profile not refreshed", "owner_id", ownerID, "reason", reason,
			"lifetime_attempts", facts.LifetimeAttempts, "sessions_since_profile", facts.SessionsSinceProfile)
		return false
	}

	if input.SessionCount, err = eventRepo.CompletedSessionCount(ctx); err != nil {
		return fail("session count", err)
	}
	counts, err := eventRepo.DiagnosisCounts(ctx, store.QueryOpts{From: time.Now().Add(-errorPatternWindow)})
	if err != nil {
		return fail("error patterns", err)
	}
	input.ErrorPatterns = errorPatterns(counts)
	if input.PreviousProfile != nil {
		input.PreviousProfile.GeneratedAt = profileTime(prevProfile)
	}
	slog.Info("session: learner profile refresh", "owner_id", ownerID, "reason", reason)
	return true
}

// skillSummary starts a skill's summary with its graph metadata.
func skillSummary(id string) lessons.MasteryDataSummary {
	s, err := skillgraph.GetSkill(id)
	if err != nil {
		return lessons.MasteryDataSummary{}
	}
	return lessons.MasteryDataSummary{Name: s.Name, Grade: s.GradeLevel}
}

// sessionSummary converts a stored session for the prompt. A session with
// no curriculum skill (an untagged quest) is not evidence about the child's
// progress, so it is dropped, and it does not count towards the cadence.
func sessionSummary(s store.SessionSummaryRecord) (lessons.SessionSummary, bool) {
	sum := lessons.SessionSummary{
		EndedAt: s.Timestamp, Questions: s.QuestionsServed, Correct: s.CorrectAnswers, Quest: s.QuestUID != "",
	}
	for _, slot := range s.Plan {
		name := skillSummary(slot.SkillID).Name
		if name == "" || slices.Contains(sum.Skills, name) {
			continue
		}
		sum.Skills = append(sum.Skills, name)
	}
	return sum, len(sum.Skills) > 0
}

// sessionsSince counts the sessions that ended after the previous profile.
// A profile with no readable time counts as old: it predates the cadence.
func sessionsSince(sessions []lessons.SessionSummary, prev *store.LearnerProfileData) int {
	at := profileTime(prev)
	if at.IsZero() {
		return len(sessions)
	}
	n := 0
	for _, s := range sessions {
		if s.EndedAt.After(at) {
			n++
		}
	}
	return n
}

func profileTime(p *store.LearnerProfileData) time.Time {
	if p == nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, p.GeneratedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

// errorPatterns keeps the curriculum rows of the diagnosis counts, already
// most frequent first, and names each misconception.
func errorPatterns(counts []store.DiagnosisCount) []lessons.ErrorPatternSummary {
	var out []lessons.ErrorPatternSummary
	for _, c := range counts {
		if len(out) == errorPatternLimit {
			break
		}
		if !curriculumSkill(c.SkillID) {
			continue
		}
		e := lessons.ErrorPatternSummary{
			SkillID: c.SkillID, SkillName: skillSummary(c.SkillID).Name, Category: c.Category, Count: c.Count,
		}
		if m := diagnosis.GetMisconception(c.MisconceptionID); m != nil {
			e.Misconception = m.Label
		}
		out = append(out, e)
	}
	return out
}

// masteryStandingChanged reports whether any skill changed state or tier
// between two snapshots. The tier matters: learn to prove is a step forward
// with no change of state. With no previous mastery data (a failed snapshot
// read, a legacy or untagged-quest snapshot) nothing is known to have
// changed. A child with no profile yet gets one by the first-profile rule.
func masteryStandingChanged(prev, next *store.MasterySnapshotData) bool {
	if prev == nil || next == nil {
		return false
	}
	for id, skm := range next.Skills {
		if !curriculumSkill(id) {
			continue
		}
		before := prev.Skills[id]
		if before == nil {
			if skm.State != "new" {
				return true
			}
			continue
		}
		if before.State != skm.State || before.CurrentTier != skm.CurrentTier {
			return true
		}
	}
	return false
}

// refreshProfile is the body of the async learner-profile refresh: generate a
// fresh profile, fold it into the latest snapshot, and — when the profile
// actually changed — append a learner-profile version event so history
// survives snapshot pruning. Every step is best-effort: a failure logs and
// never breaks the session save that spawned the refresh. It must log,
// though — an unlogged return here is a profile that silently never updated,
// with nothing in production to say so.
func refreshProfile(ctx context.Context, ownerID string, snapRepo store.SnapshotRepo, eventRepo store.EventRepo, compressor *lessons.Compressor, input lessons.ProfileInput, prevProfile *store.LearnerProfileData) {
	profile, err := compressor.GenerateProfile(ctx, input)
	if err != nil {
		slog.Error("session: generate learner profile", "owner_id", ownerID, "err", err)
		return
	}
	if profile == nil {
		slog.Error("session: generate learner profile returned no profile", "owner_id", ownerID)
		return
	}
	newProfile := &store.LearnerProfileData{
		Summary:    profile.Summary,
		Strengths:  profile.Strengths,
		Weaknesses: profile.Weaknesses,
		Patterns:   profile.Patterns,
		// Sub-second precision: the cadence compares this with session end
		// times, and a value rounded down to the second can fall before the
		// end of the session that triggered it.
		GeneratedAt: profile.GeneratedAt.UTC().Format(time.RFC3339Nano),
	}
	// In place, rather than loading the snapshot and saving a modified copy:
	// this goroutine outlives the play slot, so a copy written back after a
	// later session's save would revert that session's progress.
	if err := snapRepo.UpdateLearnerProfile(ctx, newProfile); err != nil {
		slog.Error("session: save learner profile", "owner_id", ownerID, "err", err)
		// Nothing was stored, so don't claim a new version below.
		return
	}
	// Version the profile as an owner-scoped event, but only when its
	// content changed — an identical regeneration is not a new version.
	if eventRepo == nil || !profileChanged(prevProfile, newProfile) {
		return
	}
	if err := eventRepo.AppendLearnerProfileEvent(ctx, store.LearnerProfileEventData{
		Summary:     newProfile.Summary,
		Strengths:   newProfile.Strengths,
		Weaknesses:  newProfile.Weaknesses,
		Patterns:    newProfile.Patterns,
		GeneratedAt: newProfile.GeneratedAt,
	}); err != nil {
		slog.Error("session: append learner profile event", "owner_id", ownerID, "err", err)
	}
}

// recoverProfile reads the newest learner-profile version from the event
// stream. It is the fallback for a failed snapshot read: losing the child's
// profile costs question personalisation until the next refresh, and the
// event stream exists precisely so profile history survives independently of
// snapshots. Best-effort — a failure here just means no carry-over.
func recoverProfile(ctx context.Context, ownerID string, eventRepo store.EventRepo) *store.LearnerProfileData {
	if eventRepo == nil {
		return nil
	}
	recs, err := eventRepo.QueryLearnerProfileEvents(ctx, store.QueryOpts{Limit: 1})
	if err != nil {
		slog.Error("session: recover learner profile from events", "owner_id", ownerID, "err", err)
		return nil
	}
	if len(recs) == 0 {
		return nil
	}
	r := recs[0]
	slog.Warn("session: recovered learner profile from event stream", "owner_id", ownerID)
	return &store.LearnerProfileData{
		Summary:     r.Summary,
		Strengths:   r.Strengths,
		Weaknesses:  r.Weaknesses,
		Patterns:    r.Patterns,
		GeneratedAt: r.GeneratedAt,
	}
}

// curriculumSkill reports whether an ID names a real skill in the graph.
//
// Not everything the session engine tracks is curriculum: an untagged quest
// plays under a synthetic "quest:<uid>" skill (specs/15-quests.md), and stale
// IDs can survive in old snapshots. Feeding those to the profiler asks the
// model to reason about an opaque token. Testing graph membership — rather
// than matching a prefix — covers both cases and needs no shared magic string.
func curriculumSkill(id string) bool {
	_, err := skillgraph.GetSkill(id)
	return err == nil
}

// snapshotFluency recomputes a skill's fluency score from the raw metrics the
// snapshot already carries. The score is derived, never stored, so this is the
// same computation mastery.SkillMastery.FluencyScore performs — done here
// against snapshot data so it cannot touch live service state.
func snapshotFluency(skm *store.SkillMasteryData) float64 {
	metrics := mastery.FluencyMetrics{
		SpeedScores: skm.SpeedScores,
		SpeedWindow: skm.SpeedWindow,
		Streak:      skm.Streak,
		StreakCap:   skm.StreakCap,
	}
	var accuracy float64
	if skm.TotalAttempts > 0 {
		accuracy = float64(skm.CorrectCount) / float64(skm.TotalAttempts)
	}
	return mastery.FluencyScore(&metrics, accuracy)
}

// profileChanged reports whether the freshly generated profile differs in
// content (summary + the three lists) from the previous one. GeneratedAt is
// deliberately ignored: regeneration alone is not a change.
func profileChanged(prev, next *store.LearnerProfileData) bool {
	if prev == nil {
		return true
	}
	return prev.Summary != next.Summary ||
		!slices.Equal(prev.Strengths, next.Strengths) ||
		!slices.Equal(prev.Weaknesses, next.Weaknesses) ||
		!slices.Equal(prev.Patterns, next.Patterns)
}
