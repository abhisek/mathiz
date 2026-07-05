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
		// Full lesson content so past tips can be revisited (the guide's
		// notebook). Empty on rows written before these fields existed.
		field.Text("explanation").Default(""),
		field.Text("worked_example").Default(""),
		field.Text("practice_text").Default(""),
		field.String("practice_answer").Default(""),
		field.Text("practice_explanation").Default(""),
	}
}

func (LessonEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("skill_id"),
	}
}
