package lessons

import (
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/store"
)

// GradePractice grades an answer to the lesson's practice question through
// the engine's single answer comparator (problemgen.CheckAnswer). Both the
// terminal session screen and the game manager grade lesson practice here.
func (l *Lesson) GradePractice(answer string) bool {
	return problemgen.CheckAnswer(answer, &problemgen.Question{
		Answer:     l.PracticeQuestion.Answer,
		AnswerType: problemgen.AnswerType(l.PracticeQuestion.AnswerType),
		Format:     problemgen.FormatNumeric,
	})
}

// EventData assembles the lesson event persisted when the lesson closes.
// The full content is included so past tips live on in the guide's notebook.
func (l *Lesson) EventData(sessionID, skillID string, attempted, correct, skipped bool) store.LessonEventData {
	return store.LessonEventData{
		SessionID:           sessionID,
		SkillID:             skillID,
		LessonTitle:         l.Title,
		PracticeAttempted:   attempted,
		PracticeCorrect:     correct,
		PracticeSkipped:     skipped,
		Explanation:         l.Explanation,
		WorkedExample:       l.WorkedExample,
		PracticeText:        l.PracticeQuestion.Text,
		PracticeAnswer:      l.PracticeQuestion.Answer,
		PracticeExplanation: l.PracticeQuestion.Explanation,
	}
}
