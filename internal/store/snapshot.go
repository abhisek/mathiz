package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/snapshot"
)

// snapshotRepo implements SnapshotRepo using the ent client.
type snapshotRepo struct {
	client *ent.Client
}

func (r *snapshotRepo) Save(ctx context.Context, snap *Snapshot) error {
	dataMap, err := snapshotDataToMap(snap.Data)
	if err != nil {
		return fmt.Errorf("marshal snapshot data: %w", err)
	}

	_, err = r.client.Snapshot.Create().
		SetSequence(snap.Sequence).
		SetTimestamp(snap.Timestamp).
		SetData(dataMap).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}
	return nil
}

func (r *snapshotRepo) Latest(ctx context.Context) (*Snapshot, error) {
	s, err := r.client.Snapshot.Query().
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
	// Find the ID threshold: get the Nth most recent snapshot.
	snapshots, err := r.client.Snapshot.Query().
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
		Where(snapshot.TimestampLTE(threshold)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("prune snapshots: %w", err)
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
