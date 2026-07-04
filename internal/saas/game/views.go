package game

// JSON view types for the treasure-map game API. The map speaks the game's
// language (islands, digging, chests); the engine's mastery/tier vocabulary
// is translated here and nowhere else.

// MapView is the full map state for a child.
type MapView struct {
	Islands []IslandView `json:"islands"`
	Gems    GemsView     `json:"gems"`
}

// IslandView is one strand rendered as an island.
type IslandView struct {
	ID    string     `json:"id"`   // strand key
	Name  string     `json:"name"` // display name
	Spots []SpotView `json:"spots"`
}

// SpotView is one skill as a dig spot.
type SpotView struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Grade         int      `json:"grade"`
	Prerequisites []string `json:"prerequisites"`

	// State: "locked" | "ready" | "digging" | "proving" | "treasure" | "sinking"
	// locked   = fog (prerequisites unmet)
	// ready    = unlocked, never attempted
	// digging  = learn tier in progress
	// proving  = prove tier in progress
	// treasure = mastered (chest open)
	// sinking  = rusty or review due — go back!
	State string `json:"state"`

	// Progress is 0..1 within the current tier (digging/proving).
	Progress float64 `json:"progress"`

	// ReviewDue marks a mastered spot whose treasure needs re-securing.
	ReviewDue bool `json:"reviewDue"`
}

// GemsView is the child's gem collection.
type GemsView struct {
	Total  int            `json:"total"`
	ByType map[string]int `json:"byType"`
}

// ExpeditionView describes a started expedition.
type ExpeditionView struct {
	ID             string `json:"id"`
	SkillID        string `json:"skillId"`
	SkillName      string `json:"skillName"`
	TotalQuestions int    `json:"totalQuestions"`
	Tier           string `json:"tier"`     // "learn" | "prove"
	Category       string `json:"category"` // "frontier" | "review" | "booster"
}

// QuestionView is one question presented to the kid.
type QuestionView struct {
	Index      int      `json:"index"` // 1-based
	Total      int      `json:"total"`
	Text       string   `json:"text"`
	Format     string   `json:"format"` // "numeric" | "multiple_choice"
	Choices    []string `json:"choices,omitempty"`
	AnswerType string   `json:"answerType"` // integer | decimal | fraction | text
	Tier       string   `json:"tier"`

	// TimeLimitSecs is set for prove-tier questions: the client shows a
	// countdown (advisory — answers are accepted after it runs out).
	TimeLimitSecs int `json:"timeLimitSecs,omitempty"`
}

// GemAwardView is a gem earned during play.
type GemAwardView struct {
	Type   string `json:"type"`
	Rarity string `json:"rarity"`
	Reason string `json:"reason"`
}

// MasteryChangeView reports a state-machine transition for celebration.
type MasteryChangeView struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// AnswerResultView is the grading response for one answer.
type AnswerResultView struct {
	Correct       bool   `json:"correct"`
	CorrectAnswer string `json:"correctAnswer"`
	Explanation   string `json:"explanation,omitempty"`
	HintAvailable bool   `json:"hintAvailable"`
	Streak        int    `json:"streak"`

	Gem              *GemAwardView      `json:"gem,omitempty"`
	Mastery          *MasteryChangeView `json:"mastery,omitempty"`
	UnlockedSkillIDs []string           `json:"unlockedSkillIds,omitempty"`

	QuestionsAnswered int  `json:"questionsAnswered"`
	TotalQuestions    int  `json:"totalQuestions"`
	Done              bool `json:"done"`

	// LessonPending means the guide is writing a micro-lesson (the kid
	// struggled twice on this skill) — poll the lesson endpoint.
	LessonPending bool `json:"lessonPending,omitempty"`

	// Summary is present when Done.
	Summary *SummaryView `json:"summary,omitempty"`
}

// LessonView is a micro-lesson from the guide. Ready=false means the guide
// is still writing — poll again.
type LessonView struct {
	Ready         bool                `json:"ready"`
	Title         string              `json:"title,omitempty"`
	Explanation   string              `json:"explanation,omitempty"`
	WorkedExample string              `json:"workedExample,omitempty"`
	Practice      *LessonPracticeView `json:"practice,omitempty"`
}

// LessonPracticeView is the lesson's try-it-yourself question.
type LessonPracticeView struct {
	Text       string `json:"text"`
	AnswerType string `json:"answerType"`
}

// LessonAnswerView grades the lesson practice attempt.
type LessonAnswerView struct {
	Correct       bool   `json:"correct"`
	CorrectAnswer string `json:"correctAnswer"`
	Explanation   string `json:"explanation,omitempty"`
}

// HintView is the revealed hint for the last answered question.
type HintView struct {
	Hint string `json:"hint"`
}

// SummaryView wraps up an expedition.
type SummaryView struct {
	Questions int            `json:"questions"`
	Correct   int            `json:"correct"`
	Accuracy  float64        `json:"accuracy"`
	Gems      []GemAwardView `json:"gems"`
	Mastered  bool           `json:"mastered"`
}
