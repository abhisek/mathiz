package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BillingState is a family space's subscription status, updated only by
// normalized billing-provider webhook events. Entitlements (credits) live
// in the credit ledger — this is display/state, not the source of spend.
type BillingState struct {
	ent.Schema
}

func (BillingState) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable(),
		field.String("family_space_id").
			Unique().
			Immutable(),
		field.String("provider").
			Default(""),
		field.String("customer_id").
			Default(""),
		field.String("subscription_id").
			Default(""),
		field.String("plan_id").
			Default(""),
		field.String("status").
			Default("none").
			Comment("none | active | past_due | canceled"),
		field.Time("current_period_end").
			Optional().
			Nillable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (BillingState) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("family_space_id"),
	}
}
