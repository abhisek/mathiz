package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
)

// EventMixin provides the base fields shared by all event types.
// Every event entity should include this mixin to get consistent
// sequence numbering and timestamping.
type EventMixin struct {
	mixin.Schema
}

func (EventMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("sequence").
			Unique().
			Immutable().
			Comment("Monotonically increasing global sequence number"),
		field.Time("timestamp").
			Default(time.Now).
			Immutable().
			Comment("UTC wall-clock time of the event"),
	}
}

func (EventMixin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("sequence"),
		index.Fields("timestamp"),
	}
}
