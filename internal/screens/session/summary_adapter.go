package session

import (
	"github.com/abhisek/mathiz/internal/screen"
	sess "github.com/abhisek/mathiz/internal/session"
	"github.com/abhisek/mathiz/internal/screens/summary"
)

// newSummaryScreenAdapter creates a summary screen from session data.
func newSummaryScreenAdapter(s *sess.SessionSummary) screen.Screen {
	return summary.New(s)
}
