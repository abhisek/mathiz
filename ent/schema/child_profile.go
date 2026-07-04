package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChildProfile is a learner inside a family space. Its UID doubles as the
// owner_id scoping key for all learning events and snapshots.
type ChildProfile struct {
	ent.Schema
}

func (ChildProfile) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable().
			Comment("Child profile ID (UUID) — also the learning data owner_id"),
		field.String("family_space_id").
			Immutable(),
		field.String("name"),
		field.Int("grade").
			Comment("School grade, 2-5"),
		field.String("pin_hash").
			Default("").
			Sensitive().
			Comment("bcrypt hash of the profile PIN, empty when no PIN is set"),
		field.Bool("archived").
			Default(false),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (ChildProfile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("family_space_id"),
	}
}
