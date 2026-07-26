package session

import (
	"context"
	"log/slog"
	"slices"
	"time"

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
		// The profile is NOT carried into the snapshot we're about to write,
		// so this read failure silently drops it until the next successful
		// refresh. Recovering it from the learner-profile event stream is
		// tracked separately; log loudly so the loss is at least visible.
		slog.Error("session: load previous snapshot for profile carry-over",
			"owner_id", ownerID, "err", err)
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

	// Async learner-profile refresh from this session's performance.
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
			sum := lessons.MasteryDataSummary{State: skm.State, FluencyScore: snapshotFluency(skm)}
			if s, err := skillgraph.GetSkill(id); err == nil {
				sum.Name = s.Name
			}
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

	eventRepo := state.EventRepo
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), profileTimeout)
		defer cancel()
		refreshProfile(bg, ownerID, snapRepo, eventRepo, compressor, input, prevProfile)
	}()
	return nil
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
	latest, err := snapRepo.Latest(ctx)
	if err != nil {
		slog.Error("session: load snapshot for learner profile", "owner_id", ownerID, "err", err)
		return
	}
	if latest == nil {
		// The caller just saved one, so this should be unreachable.
		slog.Error("session: no snapshot to attach learner profile to", "owner_id", ownerID)
		return
	}
	newProfile := &store.LearnerProfileData{
		Summary:     profile.Summary,
		Strengths:   profile.Strengths,
		Weaknesses:  profile.Weaknesses,
		Patterns:    profile.Patterns,
		GeneratedAt: profile.GeneratedAt.UTC().Format(time.RFC3339),
	}
	latest.Data.LearnerProfile = newProfile
	if err := snapRepo.Save(ctx, &store.Snapshot{Timestamp: time.Now(), Data: latest.Data}); err != nil {
		slog.Error("session: save learner profile", "owner_id", ownerID, "err", err)
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
