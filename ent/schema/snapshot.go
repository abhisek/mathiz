package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Snapshot captures the full learner state at a point in time,
// enabling fast restore without replaying the entire event log.
type Snapshot struct {
	ent.Schema
}

func (Snapshot) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("sequence").
			Comment("Event sequence number at the time of snapshot"),
		field.Time("timestamp").
			Default(time.Now).
			Comment("When the snapshot was taken"),
		field.JSON("data", map[string]any{}).
			Comment("Full learner state as JSON"),
	}
}

func (Snapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("timestamp"),
		index.Fields("sequence"),
	}
}
