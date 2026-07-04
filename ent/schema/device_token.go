package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DeviceToken is a long-lived bearer credential for a child device, minted by
// redeeming a join code. Only the SHA-256 hash of the token is stored.
type DeviceToken struct {
	ent.Schema
}

func (DeviceToken) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable().
			Comment("Device token ID (UUID)"),
		field.String("child_profile_id").
			Immutable(),
		field.String("family_space_id").
			Immutable(),
		field.String("token_hash").
			Unique().
			Immutable().
			Sensitive().
			Comment("Hex SHA-256 of the plaintext token"),
		field.String("device_label").
			Default(""),
		field.Bool("revoked").
			Default(false),
		field.Time("last_used_at").
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (DeviceToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("child_profile_id"),
		index.Fields("token_hash"),
	}
}
