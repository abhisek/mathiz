package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// FamilyMember links a parent account to a family space with a role
// (specs/12-saas.md, "Co-parents"). Control-plane, no edges (house style).
// account_id is UNIQUE: one family per account in v1. The space's
// owner_account_id column remains the owner anchor; the owner's member row
// is backfilled lazily on /me.
type FamilyMember struct {
	ent.Schema
}

func (FamilyMember) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable().
			Comment("Membership ID (UUID)"),
		field.String("family_space_id").
			Immutable(),
		field.String("account_id").
			Unique().
			Immutable().
			Comment("Member account UID (unique: one family per account in v1)"),
		field.String("role").
			Comment("owner | parent"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (FamilyMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("family_space_id"),
		index.Fields("account_id"),
	}
}
