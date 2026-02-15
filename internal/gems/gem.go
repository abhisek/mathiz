package gems

import "time"

// GemAward represents a single gem earned.
type GemAward struct {
	Type      GemType
	Rarity    Rarity
	SkillID   string // empty for session/streak gems
	SkillName string // empty for session/streak gems
	SessionID string
	Reason    string // human-readable reason, e.g. "Mastered 3-Digit Addition"
	AwardedAt time.Time
}
