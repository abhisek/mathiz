package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// FamilySpace groups a parent account with its child profiles.
type FamilySpace struct {
	ent.Schema
}

func (FamilySpace) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable().
			Comment("Family space ID (UUID)"),
		field.String("owner_account_id").
			Immutable().
			Comment("Account UID of the parent who owns this space"),
		field.String("name"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (FamilySpace) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("owner_account_id"),
	}
}
