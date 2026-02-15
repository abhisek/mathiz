package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// LessonEvent records that a micro-lesson was generated and shown.
type LessonEvent struct {
	ent.Schema
}

func (LessonEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{EventMixin{}}
}

func (LessonEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_id").NotEmpty(),
		field.String("skill_id").NotEmpty(),
		field.String("lesson_title").NotEmpty(),
		field.Bool("practice_attempted"),
		field.Bool("practice_correct"),
		field.Bool("practice_skipped"),
	}
}

func (LessonEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("skill_id"),
	}
}
