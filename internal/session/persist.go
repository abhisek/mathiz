package session

import (
	"context"
	"log"
	"time"

	"github.com/abhisek/mathiz/internal/lessons"
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
func SaveSnapshotWithProfile(ctx context.Context, snapRepo store.SnapshotRepo, compressor *lessons.Compressor, state *SessionState, snapData store.SnapshotData) error {
	// One Latest fetch serves both the carried-over profile and the
	// PreviousProfile compression input below.
	var prevProfile *store.LearnerProfileData
	if prev, err := snapRepo.Latest(ctx); err == nil && prev != nil {
		prevProfile = prev.Data.LearnerProfile
	}
	snapData.LearnerProfile = prevProfile

	if err := snapRepo.Save(ctx, &store.Snapshot{Timestamp: time.Now(), Data: snapData}); err != nil {
		return err
	}
	_ = snapRepo.Prune(ctx, snapshotKeep)

	if compressor == nil {
		return nil
	}

	// Async learner-profile refresh from this session's performance.
	input := lessons.ProfileInput{
		PerSkillResults: make(map[string]lessons.SkillResultSummary),
		MasteryData:     make(map[string]lessons.MasteryDataSummary),
		ErrorHistory:    make(map[string][]string),
	}
	for id, r := range state.PerSkillResults {
		input.PerSkillResults[id] = lessons.SkillResultSummary{Attempted: r.Attempted, Correct: r.Correct}
	}
	if snapData.Mastery != nil {
		for id, skm := range snapData.Mastery.Skills {
			input.MasteryData[id] = lessons.MasteryDataSummary{State: skm.State}
		}
	}
	state.ErrorMu.Lock()
	for id, errs := range state.RecentErrors {
		input.ErrorHistory[id] = append([]string(nil), errs...)
	}
	state.ErrorMu.Unlock()
	if prevProfile != nil {
		input.PreviousProfile = &lessons.LearnerProfile{
			Summary: prevProfile.Summary, Strengths: prevProfile.Strengths,
			Weaknesses: prevProfile.Weaknesses, Patterns: prevProfile.Patterns,
		}
	}

	go func() {
		bg, cancel := context.WithTimeout(context.Background(), profileTimeout)
		defer cancel()
		profile, err := compressor.GenerateProfile(bg, input)
		if err != nil || profile == nil {
			return
		}
		latest, err := snapRepo.Latest(bg)
		if err != nil || latest == nil {
			return
		}
		latest.Data.LearnerProfile = &store.LearnerProfileData{
			Summary:     profile.Summary,
			Strengths:   profile.Strengths,
			Weaknesses:  profile.Weaknesses,
			Patterns:    profile.Patterns,
			GeneratedAt: profile.GeneratedAt.UTC().Format(time.RFC3339),
		}
		if err := snapRepo.Save(bg, &store.Snapshot{Timestamp: time.Now(), Data: latest.Data}); err != nil {
			log.Printf("session: save learner profile: %v", err)
		}
	}()
	return nil
}
