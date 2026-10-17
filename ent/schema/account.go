package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Account is a parent identity provisioned from a verified auth-provider
// JWT (specs/17-auth-providers.md). Accounts are created implicitly on the
// first authenticated API call. The provider's subject is unique only
// within that provider, so the pair is the key; email never is.
type Account struct {
	ent.Schema
}

func (Account) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable().
			Comment("Internal account ID (UUID)"),
		field.String("auth_provider").
			NotEmpty().
			Comment("Auth provider preset that issued the identity (MATHIZ_AUTH_PROVIDER)"),
		field.String("auth_subject").
			NotEmpty().
			Comment("Provider user ID (JWT sub claim); opaque, unique per provider"),
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
		index.Fields("auth_provider", "auth_subject").Unique(),
	}
}
