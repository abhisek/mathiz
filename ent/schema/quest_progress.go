package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// QuestProgress records one child's result on one quest question.
// Control-plane by design (specs/15-quests.md §2): quest progress rows avoid
// widening the event-sourced EventRepo interface. Once correct, a question
// stays correct — expeditions serve only not-yet-correct questions.
type QuestProgress struct {
	ent.Schema
}

func (QuestProgress) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable().
			Comment("Progress row ID (UUID)"),
		field.String("quest_uid").
			Immutable(),
		field.String("child_uid").
			Immutable(),
		field.String("question_uid").
			Immutable(),
		field.Bool("correct").
			Default(false),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (QuestProgress) Indexes() []ent.Index {
	return []ent.Index{
		// One row per child per question; upserts flip correct at most once.
		index.Fields("quest_uid", "child_uid", "question_uid").Unique(),
		index.Fields("quest_uid", "child_uid"),
	}
}
