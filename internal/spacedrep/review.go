package spacedrep

import "time"

// ReviewState holds the spaced repetition state for a single skill.
type ReviewState struct {
	SkillID         string    `json:"skill_id"`
	Stage           int       `json:"stage"`
	NextReviewDate  time.Time `json:"next_review_date"`
	ConsecutiveHits int       `json:"consecutive_hits"`
	Graduated       bool      `json:"graduated"`
	LastReviewDate  time.Time `json:"last_review_date"`
}

// IsDue returns true if the skill is due for review (at or past the review date).
func (rs *ReviewState) IsDue(now time.Time) bool {
	return !now.Before(rs.NextReviewDate)
}

// OverdueDays returns how many days past due the skill is. Returns 0 if not yet due.
func (rs *ReviewState) OverdueDays(now time.Time) float64 {
	if now.Before(rs.NextReviewDate) {
		return 0
	}
	return now.Sub(rs.NextReviewDate).Hours() / 24.0
}

// IsRustyThreshold returns true if the skill has exceeded its grace period
// and should be marked rusty.
func (rs *ReviewState) IsRustyThreshold(now time.Time) bool {
	if !rs.IsDue(now) {
		return false
	}
	interval := rs.CurrentIntervalDays()
	graceHours := float64(interval) * 0.5 * 24.0
	threshold := rs.NextReviewDate.Add(time.Duration(graceHours * float64(time.Hour)))
	return now.After(threshold)
}

// CurrentIntervalDays returns the current interval in days.
func (rs *ReviewState) CurrentIntervalDays() int {
	if rs.Graduated {
		return GraduatedIntervalDays
	}
	if rs.Stage >= len(BaseIntervals) {
		return BaseIntervals[len(BaseIntervals)-1]
	}
	return BaseIntervals[rs.Stage]
}

// ReviewStatus describes a skill's review status for display.
type ReviewStatus string

const (
	ReviewNotDue    ReviewStatus = "not_due"
	ReviewDue       ReviewStatus = "due"
	ReviewOverdue   ReviewStatus = "overdue"
	ReviewGraduated ReviewStatus = "graduated"
)

// Status returns the review status for UI display.
func (rs *ReviewState) Status(now time.Time) ReviewStatus {
	if rs.Graduated && !rs.IsDue(now) {
		return ReviewGraduated
	}
	if rs.IsRustyThreshold(now) {
		return ReviewOverdue
	}
	if rs.IsDue(now) {
		return ReviewDue
	}
	if rs.Graduated {
		return ReviewGraduated
	}
	return ReviewNotDue
}

// DaysUntilReview returns the number of days until the next review.
// Returns 0 if already due.
func (rs *ReviewState) DaysUntilReview(now time.Time) int {
	if rs.IsDue(now) {
		return 0
	}
	return int(rs.NextReviewDate.Sub(now).Hours()/24.0) + 1
}
