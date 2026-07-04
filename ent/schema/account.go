package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Account is a parent identity provisioned from a verified Supabase JWT.
// Accounts are created implicitly on first authenticated API call.
type Account struct {
	ent.Schema
}

func (Account) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable().
			Comment("Internal account ID (UUID)"),
		field.String("supabase_user_id").
			Unique().
			Immutable().
			Comment("Supabase auth user ID (JWT sub claim)"),
		field.String("email").
			Default(""),
		field.String("display_name").
			Default(""),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (Account) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("supabase_user_id"),
	}
}
