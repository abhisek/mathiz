package spacedrep

import (
	"testing"
	"time"
)

func TestIsDue_BeforeDate(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	rs := &ReviewState{NextReviewDate: now.Add(24 * time.Hour)}
	if rs.IsDue(now) {
		t.Error("expected not due before review date")
	}
}

func TestIsDue_OnDate(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	rs := &ReviewState{NextReviewDate: now}
	if !rs.IsDue(now) {
		t.Error("expected due on review date")
	}
}

func TestIsDue_AfterDate(t *testing.T) {
	now := time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC)
	rs := &ReviewState{NextReviewDate: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)}
	if !rs.IsDue(now) {
		t.Error("expected due after review date")
	}
}

func TestOverdueDays_NotDue(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	rs := &ReviewState{NextReviewDate: now.Add(48 * time.Hour)}
	got := rs.OverdueDays(now)
	if got != 0 {
		t.Errorf("OverdueDays() = %f, want 0", got)
	}
}

func TestOverdueDays_ThreeDaysOverdue(t *testing.T) {
	reviewDate := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	now := reviewDate.Add(3 * 24 * time.Hour)
	rs := &ReviewState{NextReviewDate: reviewDate}
	got := rs.OverdueDays(now)
	if got < 2.99 || got > 3.01 {
		t.Errorf("OverdueDays() = %f, want ~3.0", got)
	}
}

func TestIsRustyThreshold_WithinGrace(t *testing.T) {
	// Stage 2 (7-day interval), 2 days overdue -> grace is 3.5 days -> not rusty
	reviewDate := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	now := reviewDate.Add(2 * 24 * time.Hour)
	rs := &ReviewState{Stage: 2, NextReviewDate: reviewDate}
	if rs.IsRustyThreshold(now) {
		t.Error("expected not rusty within grace period")
	}
}

func TestIsRustyThreshold_PastGrace(t *testing.T) {
	// Stage 2 (7-day interval), 4 days overdue -> grace is 3.5 days -> rusty
	reviewDate := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	now := reviewDate.Add(4 * 24 * time.Hour)
	rs := &ReviewState{Stage: 2, NextReviewDate: reviewDate}
	if !rs.IsRustyThreshold(now) {
		t.Error("expected rusty past grace period")
	}
}

func TestIsRustyThreshold_Stage0(t *testing.T) {
	// Stage 0 (1-day interval), 1 day overdue -> grace is 0.5 days -> rusty
	reviewDate := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	now := reviewDate.Add(1 * 24 * time.Hour)
	rs := &ReviewState{Stage: 0, NextReviewDate: reviewDate}
	if !rs.IsRustyThreshold(now) {
		t.Error("expected rusty for stage 0 after 1 day overdue")
	}
}

func TestIsRustyThreshold_Graduated_NotRusty(t *testing.T) {
	// Graduated (90-day interval), 30 days overdue -> grace is 45 days -> not rusty
	reviewDate := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	now := reviewDate.Add(30 * 24 * time.Hour)
	rs := &ReviewState{Stage: 6, Graduated: true, NextReviewDate: reviewDate}
	if rs.IsRustyThreshold(now) {
		t.Error("expected not rusty for graduated within 45-day grace")
	}
}

func TestIsRustyThreshold_Graduated_Rusty(t *testing.T) {
	// Graduated (90-day interval), 50 days overdue -> grace is 45 days -> rusty
	reviewDate := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	now := reviewDate.Add(50 * 24 * time.Hour)
	rs := &ReviewState{Stage: 6, Graduated: true, NextReviewDate: reviewDate}
	if !rs.IsRustyThreshold(now) {
		t.Error("expected rusty for graduated past 45-day grace")
	}
}

func TestStatus_NotDue(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	rs := &ReviewState{Stage: 2, NextReviewDate: now.Add(5 * 24 * time.Hour)}
	got := rs.Status(now)
	if got != ReviewNotDue {
		t.Errorf("Status() = %q, want %q", got, ReviewNotDue)
	}
}

func TestStatus_Due(t *testing.T) {
	// Past review date, within grace period
	reviewDate := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	now := reviewDate.Add(1 * 24 * time.Hour) // 1 day overdue, stage 2 grace is 3.5 days
	rs := &ReviewState{Stage: 2, NextReviewDate: reviewDate}
	got := rs.Status(now)
	if got != ReviewDue {
		t.Errorf("Status() = %q, want %q", got, ReviewDue)
	}
}

func TestStatus_Overdue(t *testing.T) {
	// Past grace period
	reviewDate := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	now := reviewDate.Add(5 * 24 * time.Hour) // 5 days overdue, stage 2 grace is 3.5 days
	rs := &ReviewState{Stage: 2, NextReviewDate: reviewDate}
	got := rs.Status(now)
	if got != ReviewOverdue {
		t.Errorf("Status() = %q, want %q", got, ReviewOverdue)
	}
}

func TestStatus_Graduated_NotDue(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	rs := &ReviewState{Stage: 6, Graduated: true, NextReviewDate: now.Add(30 * 24 * time.Hour)}
	got := rs.Status(now)
	if got != ReviewGraduated {
		t.Errorf("Status() = %q, want %q", got, ReviewGraduated)
	}
}

func TestStatus_Graduated_Due(t *testing.T) {
	// Graduated but due (within grace) -> ReviewDue
	reviewDate := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	now := reviewDate.Add(10 * 24 * time.Hour) // 10 days overdue, 45-day grace
	rs := &ReviewState{Stage: 6, Graduated: true, NextReviewDate: reviewDate}
	got := rs.Status(now)
	if got != ReviewDue {
		t.Errorf("Status() = %q, want %q", got, ReviewDue)
	}
}

func TestDaysUntilReview_FutureDue(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	// 4.5 days in the future -> int(4.5) + 1 = 5
	rs := &ReviewState{NextReviewDate: now.Add(108 * time.Hour)} // 4.5 days
	got := rs.DaysUntilReview(now)
	if got != 5 {
		t.Errorf("DaysUntilReview() = %d, want 5", got)
	}
}

func TestDaysUntilReview_AlreadyDue(t *testing.T) {
	now := time.Date(2025, 1, 5, 12, 0, 0, 0, time.UTC)
	rs := &ReviewState{NextReviewDate: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)}
	got := rs.DaysUntilReview(now)
	if got != 0 {
		t.Errorf("DaysUntilReview() = %d, want 0", got)
	}
}
