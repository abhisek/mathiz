package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MasteryEvent records a mastery state transition for audit and analytics.
type MasteryEvent struct {
	ent.Schema
}

func (MasteryEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{EventMixin{}}
}

func (MasteryEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("skill_id").NotEmpty(),
		field.String("from_state").NotEmpty(),
		field.String("to_state").NotEmpty(),
		field.String("trigger").NotEmpty(),
		field.Float("fluency_score"),
		field.String("session_id").Optional(),
	}
}

func (MasteryEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("skill_id"),
	}
}
