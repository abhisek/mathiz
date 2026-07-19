package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// QuestQuestion is one authored question in a quest's ordered flat list
// (specs/15-quests.md §2 — a set, not a graph; no prerequisites/branching).
type QuestQuestion struct {
	ent.Schema
}

func (QuestQuestion) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable().
			Comment("Question ID (UUID)"),
		field.String("quest_uid").
			Immutable(),
		field.Int("position").
			Comment("Order within the quest (0-based, gaps allowed)"),
		field.Text("text"),
		field.String("answer"),
		field.String("answer_type").
			Comment("integer | decimal | fraction | text"),
		field.String("format").
			Comment("numeric | multiple_choice"),
		field.JSON("choices", []string{}).
			Optional().
			Comment("Options for multiple_choice format"),
		field.Text("hint").
			Default(""),
		field.Text("explanation").
			Default(""),
		field.String("client_key").
			Default("").
			Immutable().
			Comment("AI-generation idempotency key; \"\" for manually authored questions"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (QuestQuestion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("quest_uid"),
		index.Fields("quest_uid", "client_key"),
	}
}
