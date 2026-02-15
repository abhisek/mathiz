package gems

import "testing"

func TestStreakRarity(t *testing.T) {
	tests := []struct {
		length int
		want   Rarity
	}{
		{5, RarityCommon},
		{7, RarityCommon},
		{9, RarityCommon},
		{10, RarityRare},
		{12, RarityRare},
		{15, RarityEpic},
		{19, RarityEpic},
		{20, RarityLegendary},
		{25, RarityLegendary},
		{100, RarityLegendary},
	}

	for _, tt := range tests {
		got := StreakRarity(tt.length)
		if got != tt.want {
			t.Errorf("StreakRarity(%d) = %q, want %q", tt.length, got, tt.want)
		}
	}
}

func TestSessionRarity(t *testing.T) {
	tests := []struct {
		accuracy float64
		want     Rarity
	}{
		{0.0, RarityCommon},
		{0.3, RarityCommon},
		{0.49, RarityCommon},
		{0.50, RarityRare},
		{0.74, RarityRare},
		{0.75, RarityEpic},
		{0.89, RarityEpic},
		{0.90, RarityLegendary},
		{1.0, RarityLegendary},
	}

	for _, tt := range tests {
		got := SessionRarity(tt.accuracy)
		if got != tt.want {
			t.Errorf("SessionRarity(%.2f) = %q, want %q", tt.accuracy, got, tt.want)
		}
	}
}

func TestAllRarities(t *testing.T) {
	rarities := AllRarities()
	if len(rarities) != 4 {
		t.Errorf("expected 4 rarities, got %d", len(rarities))
	}
	if rarities[0] != RarityCommon || rarities[3] != RarityLegendary {
		t.Errorf("unexpected order: %v", rarities)
	}
}

func TestRarity_DisplayName(t *testing.T) {
	tests := []struct {
		rarity Rarity
		want   string
	}{
		{RarityCommon, "Common"},
		{RarityRare, "Rare"},
		{RarityEpic, "Epic"},
		{RarityLegendary, "Legendary"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		got := tt.rarity.DisplayName()
		if got != tt.want {
			t.Errorf("Rarity(%q).DisplayName() = %q, want %q", tt.rarity, got, tt.want)
		}
	}
}
