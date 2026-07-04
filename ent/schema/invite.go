package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Invite is a family-space join code. A child redeems it (with their profile
// PIN, if set) to obtain a device token. Codes are low-privilege by design:
// redemption only ever yields child-level access, so the human-friendly code
// is stored in plaintext for display in the parent dashboard.
type Invite struct {
	ent.Schema
}

func (Invite) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable().
			Comment("Invite ID (UUID)"),
		field.String("family_space_id").
			Immutable(),
		field.String("code").
			Unique().
			Immutable().
			Comment("Human-friendly join code, e.g. TIGER-4207"),
		field.Time("expires_at"),
		field.Bool("revoked").
			Default(false),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (Invite) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("family_space_id"),
		index.Fields("code"),
	}
}
