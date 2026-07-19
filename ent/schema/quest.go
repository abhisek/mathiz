package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Quest is a parent-authored one-off question set (specs/15-quests.md).
// Control-plane, family-scoped like invites — NOT event-sourced. A quest
// targets one child (child_uid) or every child in the space (child_uid "").
// An optional skill_id tag routes answers through the normal mastery /
// spaced-rep services; untagged quests leave the skill graph untouched.
type Quest struct {
	ent.Schema
}

func (Quest) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable().
			Comment("Quest ID (UUID)"),
		field.String("family_space_id").
			Immutable(),
		field.String("name"),
		field.String("emoji").
			Default("").
			Comment("Optional display emoji for the map card"),
		field.String("skill_id").
			Default("").
			Comment("Optional skillgraph tag; \"\" = untagged (no mastery feed)"),
		field.String("child_uid").
			Default("").
			Comment("Target child profile UID; \"\" = all children in the space"),
		field.String("status").
			Default("draft").
			Comment("draft | active | archived"),
		field.String("created_by").
			Default("").
			Immutable().
			Comment("Account UID of the authoring parent (\"\" for pre-membership quests)"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (Quest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("family_space_id"),
		index.Fields("family_space_id", "status"),
	}
}
