package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AnswerEvent records a single answer event within a session.
type AnswerEvent struct {
	ent.Schema
}

func (AnswerEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{EventMixin{}}
}

func (AnswerEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_id").
			NotEmpty().
			Comment("Links to SessionEvent"),
		field.String("skill_id").
			NotEmpty().
			Comment("Skill this question was for"),
		field.String("tier").
			NotEmpty().
			Comment("learn or prove"),
		field.String("category").
			NotEmpty().
			Comment("frontier, review, or booster"),
		field.String("question_text").
			NotEmpty().
			Comment("The question shown"),
		field.String("correct_answer").
			NotEmpty().
			Comment("The canonical correct answer"),
		field.String("learner_answer").
			NotEmpty().
			Comment("What the learner entered"),
		field.Bool("correct").
			Comment("Whether the answer was correct"),
		field.Int("time_ms").
			Comment("Milliseconds to answer"),
		field.String("answer_format").
			NotEmpty().
			Comment("numeric or multiple_choice"),
	}
}

func (AnswerEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("skill_id"),
		index.Fields("correct"),
	}
}
