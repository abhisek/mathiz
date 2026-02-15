package gems

// StreakThreshold is a streak length that awards a gem.
const BaseStreakThreshold = 5

// NextStreakThreshold returns the next streak milestone above the current streak length.
func NextStreakThreshold(current int) int {
	thresholds := []int{5, 10, 15, 20}
	for _, t := range thresholds {
		if t > current {
			return t
		}
	}
	// Beyond 20, award every 5.
	return ((current / 5) + 1) * 5
}
