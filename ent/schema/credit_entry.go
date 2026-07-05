package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CreditEntry is one row in a family space's credit ledger: grants
// (starter/plan/topup, positive amount with a remaining counter and optional
// expiry) and debits (negative amount, consumed FIFO from grants). The
// unique source makes replayed webhooks and retried charges structurally
// idempotent.
type CreditEntry struct {
	ent.Schema
}

func (CreditEntry) Fields() []ent.Field {
	return []ent.Field{
		field.String("uid").
			Unique().
			Immutable().
			Comment("Ledger entry ID (UUID)"),
		field.String("family_space_id").
			Immutable(),
		field.String("kind").
			Immutable().
			Comment("starter | plan | topup | debit"),
		field.Int("amount").
			Immutable().
			Comment("Positive for grants, negative for debits"),
		field.Int("remaining").
			Default(0).
			Comment("Unconsumed credits on grant entries; 0 for debits"),
		field.String("source").
			Unique().
			Immutable().
			Comment("Idempotency key, e.g. starter:<space>, sub:<eventID>, expedition:<sessionID>"),
		field.Time("expires_at").
			Optional().
			Nillable().
			Immutable().
			Comment("Grant expiry; nil = never expires"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (CreditEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("family_space_id"),
		index.Fields("family_space_id", "remaining"),
	}
}
