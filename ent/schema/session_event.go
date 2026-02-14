package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SessionEvent records session lifecycle events (start/end).
type SessionEvent struct {
	ent.Schema
}

func (SessionEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{EventMixin{}}
}

// PlanSlotSummary is the serialized form of a plan slot for persistence.
type PlanSlotSummary struct {
	SkillID  string `json:"skill_id"`
	Tier     string `json:"tier"`
	Category string `json:"category"`
}

func (SessionEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_id").
			NotEmpty().
			Comment("UUID grouping events in a session"),
		field.String("action").
			NotEmpty().
			Comment("start or end"),
		field.Int("questions_served").
			Default(0).
			Comment("Total questions (on end only)"),
		field.Int("correct_answers").
			Default(0).
			Comment("Total correct (on end only)"),
		field.Int("duration_secs").
			Default(0).
			Comment("Actual duration in seconds (on end only)"),
		field.JSON("plan_summary", []PlanSlotSummary{}).
			Optional().
			Comment("Serialized plan (on start only)"),
	}
}

func (SessionEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("action"),
	}
}
