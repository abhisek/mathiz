package billing

import "os"

// Plan is a purchasable item: a monthly subscription (MonthlyCredits > 0)
// or a one-time top-up pack (TopupCredits > 0). The catalog lives here;
// provider price IDs come from env so the same build works against any
// Stripe/Paddle account.
type Plan struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	PriceUSDCents  int    `json:"priceUsdCents"`
	MonthlyCredits int    `json:"monthlyCredits,omitempty"`
	TopupCredits   int    `json:"topupCredits,omitempty"`
	Blurb          string `json:"blurb"`

	// ProviderPriceID maps this plan to the provider's price object
	// (MATHIZ_BILLING_PRICE_<ID>). Unused by the fake provider.
	ProviderPriceID string `json:"-"`
}

// Subscription reports whether the plan is recurring.
func (p Plan) Subscription() bool { return p.MonthlyCredits > 0 }

// Plans is the catalog. 1 credit = 1 expedition (5 AI questions, hints and
// micro-lessons included).
func Plans() []Plan {
	return []Plan{
		{
			ID: "explorer", Name: "Explorer", PriceUSDCents: 500,
			MonthlyCredits:  150,
			Blurb:           "150 expeditions every month — steady practice for one or two kids.",
			ProviderPriceID: os.Getenv("MATHIZ_BILLING_PRICE_EXPLORER"),
		},
		{
			ID: "voyager", Name: "Voyager", PriceUSDCents: 1000,
			MonthlyCredits:  400,
			Blurb:           "400 expeditions monthly at a better rate — room for the whole crew.",
			ProviderPriceID: os.Getenv("MATHIZ_BILLING_PRICE_VOYAGER"),
		},
		{
			ID: "armada", Name: "Armada", PriceUSDCents: 2000,
			MonthlyCredits:  1000,
			Blurb:           "1,000 expeditions monthly at the best rate for big families.",
			ProviderPriceID: os.Getenv("MATHIZ_BILLING_PRICE_ARMADA"),
		},
		{
			ID: "topup-100", Name: "Top-up pack", PriceUSDCents: 500,
			TopupCredits:    100,
			Blurb:           "100 extra expeditions, never expire.",
			ProviderPriceID: os.Getenv("MATHIZ_BILLING_PRICE_TOPUP_100"),
		},
	}
}

// PlanByID looks a plan up in the catalog.
func PlanByID(id string) (Plan, bool) {
	for _, p := range Plans() {
		if p.ID == id {
			return p, true
		}
	}
	return Plan{}, false
}
