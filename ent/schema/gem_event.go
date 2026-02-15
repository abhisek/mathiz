package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// GemEvent records a gem award.
type GemEvent struct {
	ent.Schema
}

func (GemEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{EventMixin{}}
}

func (GemEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("gem_type").NotEmpty(),
		field.String("rarity").NotEmpty(),
		field.String("skill_id").Optional().Nillable(),
		field.String("skill_name").Optional().Nillable(),
		field.String("session_id").NotEmpty(),
		field.String("reason").NotEmpty(),
	}
}

func (GemEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("gem_type"),
		index.Fields("session_id"),
		index.Fields("rarity"),
	}
}
