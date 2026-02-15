package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// HintEvent records that a hint was shown to the learner.
type HintEvent struct {
	ent.Schema
}

func (HintEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{EventMixin{}}
}

func (HintEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_id").NotEmpty(),
		field.String("skill_id").NotEmpty(),
		field.String("question_text").NotEmpty(),
		field.String("hint_text").NotEmpty(),
	}
}

func (HintEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("skill_id"),
	}
}
