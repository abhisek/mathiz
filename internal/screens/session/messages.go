package session

import (
	"time"

	"github.com/abhisek/mathiz/internal/problemgen"
	sess "github.com/abhisek/mathiz/internal/session"
)

// questionReadyMsg is sent when a question has been generated.
type questionReadyMsg struct {
	Question *problemgen.Question
	Err      error
}

// questionGenFailedMsg is sent when question generation fails after retries.
type questionGenFailedMsg struct {
	Err error
}

// timerTickMsg is sent every second to update the countdown.
type timerTickMsg time.Time

// spinnerTickMsg is sent at short intervals to animate the loading spinner.
type spinnerTickMsg time.Time

// feedbackDoneMsg is sent when the feedback display period ends.
type feedbackDoneMsg struct{}

// sessionInitMsg is sent when session initialization (plan building) is complete.
type sessionInitMsg struct {
	State *sess.SessionState
	Err   error
}

// sessionEndMsg is sent to trigger the session end flow.
type sessionEndMsg struct{}

// persistAnswerMsg is sent to confirm answer persistence completed.
type persistAnswerMsg struct {
	Err error
}
