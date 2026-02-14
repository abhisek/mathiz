package spacedrep

// BaseIntervals defines the expanding interval schedule in days.
// Stage 0 = first review after mastery.
var BaseIntervals = []int{1, 3, 7, 14, 30, 60}

// MaxStage is the highest stage index in BaseIntervals.
const MaxStage = 5

// GraduationStage is the stage at which a skill graduates.
// A skill graduates after completing all 6 stages (0-5) successfully.
const GraduationStage = 6

// GraduatedIntervalDays is the review interval for graduated skills.
const GraduatedIntervalDays = 90
