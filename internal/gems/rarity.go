package gems

// Rarity represents the difficulty tier of a gem.
type Rarity string

const (
	RarityCommon    Rarity = "common"
	RarityRare      Rarity = "rare"
	RarityEpic      Rarity = "epic"
	RarityLegendary Rarity = "legendary"
)

// AllRarities returns all rarities in order from lowest to highest.
func AllRarities() []Rarity {
	return []Rarity{RarityCommon, RarityRare, RarityEpic, RarityLegendary}
}

// DisplayName returns a human-readable label for the rarity.
func (r Rarity) DisplayName() string {
	switch r {
	case RarityCommon:
		return "Common"
	case RarityRare:
		return "Rare"
	case RarityEpic:
		return "Epic"
	case RarityLegendary:
		return "Legendary"
	default:
		return string(r)
	}
}

// StreakRarity returns the rarity for a given streak length.
func StreakRarity(length int) Rarity {
	switch {
	case length >= 20:
		return RarityLegendary
	case length >= 15:
		return RarityEpic
	case length >= 10:
		return RarityRare
	default:
		return RarityCommon
	}
}

// SessionRarity returns the rarity for a given session accuracy (0.0-1.0).
func SessionRarity(accuracy float64) Rarity {
	switch {
	case accuracy >= 0.90:
		return RarityLegendary
	case accuracy >= 0.75:
		return RarityEpic
	case accuracy >= 0.50:
		return RarityRare
	default:
		return RarityCommon
	}
}
