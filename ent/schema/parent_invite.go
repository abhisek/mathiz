package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ParentInvite is an email-recorded co-parent invitation (specs/12-saas.md,
// "Co-parents"). No email is ever sent: the invitee signs in normally and
// /me surfaces the pending invite when the account email matches. A typo'd
// email simply never matches.
type ParentInvite struct {
	ent.Schema
}

func (ParentInvite) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable().
			Comment("Parent invite ID (UUID)"),
		field.String("family_space_id").
			Immutable(),
		field.String("email").
			Immutable().
			Comment("Invitee email, stored lowercased"),
		field.String("status").
			Default("pending").
			Comment("pending | accepted | revoked"),
		field.String("created_by").
			Immutable().
			Comment("Account UID of the inviting owner"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (ParentInvite) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("family_space_id"),
		index.Fields("email", "status"),
	}
}
