package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/snapshot"
)

// snapshotRepo implements SnapshotRepo using the ent client, scoped to a
// single owner (learner).
type snapshotRepo struct {
	client *ent.Client
	owner  string
}

// scope stamps the repo's owner into ctx so the store-level owner guard
// (see ownerguard.go) scopes every ent call made during the request. Every
// exported repo method must wrap its ctx with this at entry.
func (r *snapshotRepo) scope(ctx context.Context) context.Context {
	return withOwner(ctx, r.owner)
}

func (r *snapshotRepo) Save(ctx context.Context, snap *Snapshot) error {
	ctx = r.scope(ctx)
	dataMap, err := snapshotDataToMap(snap.Data)
	if err != nil {
		return fmt.Errorf("marshal snapshot data: %w", err)
	}

	_, err = r.client.Snapshot.Create().
		SetSequence(snap.Sequence).
		SetOwnerID(r.owner).
		SetTimestamp(snap.Timestamp).
		SetData(dataMap).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}
	return nil
}

func (r *snapshotRepo) Latest(ctx context.Context) (*Snapshot, error) {
	ctx = r.scope(ctx)
	s, err := r.client.Snapshot.Query().
		Where(snapshot.OwnerID(r.owner)).
		Order(ent.Desc(snapshot.FieldTimestamp)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("query latest snapshot: %w", err)
	}
	return entSnapshotToSnapshot(s)
}

func (r *snapshotRepo) Prune(ctx context.Context, keep int) error {
	ctx = r.scope(ctx)
	// Find the ID threshold: get the Nth most recent snapshot.
	snapshots, err := r.client.Snapshot.Query().
		Where(snapshot.OwnerID(r.owner)).
		Order(ent.Desc(snapshot.FieldTimestamp)).
		Offset(keep).
		Limit(1).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query snapshots for prune: %w", err)
	}
	if len(snapshots) == 0 {
		return nil // fewer than keep snapshots exist
	}

	threshold := snapshots[0].Timestamp
	_, err = r.client.Snapshot.Delete().
		Where(snapshot.OwnerID(r.owner), snapshot.TimestampLTE(threshold)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("prune snapshots: %w", err)
	}
	return nil
}

// UpdateLearnerProfile replaces the learner profile on the owner's most recent
// snapshot in place, leaving every other field untouched.
//
// The async profile refresh used to do this by loading the latest snapshot and
// Save()ing a modified copy. Save appends a new row and Latest is "max
// timestamp", so a snapshot read before a concurrent session's save would be
// written back as the newest one — silently reverting that session's mastery,
// spaced-rep schedule and gems. It also doubled the row count per session,
// halving how much history Prune retained. Updating the row in place removes
// both problems: progress fields are never rewritten from a stale read, and no
// row is added.
//
// A narrow window remains between reading the latest row and updating it (a
// concurrent save could make a newer row latest, leaving the profile on the
// previous one). That costs a profile update, not a learner's progress.
func (r *snapshotRepo) UpdateLearnerProfile(ctx context.Context, profile *LearnerProfileData) error {
	ctx = r.scope(ctx)
	latest, err := r.client.Snapshot.Query().
		Where(snapshot.OwnerID(r.owner)).
		Order(ent.Desc(snapshot.FieldTimestamp)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrNoSnapshot
		}
		return fmt.Errorf("query latest snapshot: %w", err)
	}

	snap, err := entSnapshotToSnapshot(latest)
	if err != nil {
		return err
	}
	snap.Data.LearnerProfile = profile
	dataMap, err := snapshotDataToMap(snap.Data)
	if err != nil {
		return fmt.Errorf("marshal snapshot data: %w", err)
	}
	if _, err := r.client.Snapshot.UpdateOneID(latest.ID).SetData(dataMap).Save(ctx); err != nil {
		return fmt.Errorf("update learner profile: %w", err)
	}
	return nil
}

// snapshotDataToMap converts SnapshotData to map[string]any for ent JSON storage.
func snapshotDataToMap(data SnapshotData) (map[string]any, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// entSnapshotToSnapshot converts an ent Snapshot to a store Snapshot.
func entSnapshotToSnapshot(s *ent.Snapshot) (*Snapshot, error) {
	b, err := json.Marshal(s.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal ent data: %w", err)
	}
	var data SnapshotData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot data: %w", err)
	}
	return &Snapshot{
		ID:        s.ID,
		Sequence:  s.Sequence,
		Timestamp: s.Timestamp,
		Data:      data,
	}, nil
}
