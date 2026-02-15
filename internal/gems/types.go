package gems

// GemType identifies the category of achievement.
type GemType string

const (
	GemMastery   GemType = "mastery"
	GemRecovery  GemType = "recovery"
	GemRetention GemType = "retention"
	GemStreak    GemType = "streak"
	GemSession   GemType = "session"
)

// AllGemTypes returns all gem types in display order.
func AllGemTypes() []GemType {
	return []GemType{GemMastery, GemRecovery, GemRetention, GemStreak, GemSession}
}

// DisplayName returns a human-readable label for the gem type.
func (t GemType) DisplayName() string {
	switch t {
	case GemMastery:
		return "Mastery"
	case GemRecovery:
		return "Recovery"
	case GemRetention:
		return "Retention"
	case GemStreak:
		return "Streak"
	case GemSession:
		return "Session"
	default:
		return string(t)
	}
}

// Icon returns the display icon for the gem type.
func (t GemType) Icon() string {
	switch t {
	case GemMastery:
		return "💎"
	case GemRecovery:
		return "🔥"
	case GemRetention:
		return "🛡️"
	case GemStreak:
		return "⚡"
	case GemSession:
		return "🏆"
	default:
		return "✦"
	}
}
