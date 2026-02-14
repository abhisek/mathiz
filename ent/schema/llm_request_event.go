package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// LLMRequestEvent records every LLM API call for cost tracking and debugging.
type LLMRequestEvent struct {
	ent.Schema
}

func (LLMRequestEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{EventMixin{}}
}

func (LLMRequestEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").
			Comment("Provider name: anthropic, openai, gemini"),
		field.String("model").
			Comment("Actual model ID used"),
		field.String("purpose").
			Comment("Consumer-provided label: question-gen, hint, lesson, diagnosis"),
		field.Int("input_tokens").
			Default(0).
			Comment("Tokens in the request"),
		field.Int("output_tokens").
			Default(0).
			Comment("Tokens in the response"),
		field.Int64("latency_ms").
			Default(0).
			Comment("Wall-clock time for the request"),
		field.Bool("success").
			Comment("Whether the request succeeded"),
		field.String("error_message").
			Default("").
			Comment("Error message if failed"),
	}
}

func (LLMRequestEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider"),
		index.Fields("purpose"),
		index.Fields("success"),
	}
}
