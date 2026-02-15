package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DiagnosisEvent records a diagnosis result for a single wrong answer.
type DiagnosisEvent struct {
	ent.Schema
}

func (DiagnosisEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{EventMixin{}}
}

func (DiagnosisEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_id").NotEmpty(),
		field.String("skill_id").NotEmpty(),
		field.String("question_text").NotEmpty(),
		field.String("correct_answer").NotEmpty(),
		field.String("learner_answer").NotEmpty(),
		field.String("category").NotEmpty(), // careless, speed-rush, misconception, unclassified
		field.String("misconception_id").Optional().Nillable(),
		field.Float("confidence"),
		field.String("classifier_name").NotEmpty(),
		field.String("reasoning").Optional().Default(""),
	}
}

func (DiagnosisEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("skill_id"),
		index.Fields("session_id"),
	}
}
