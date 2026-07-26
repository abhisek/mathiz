package lessons

import (
	"errors"
	"fmt"
	"strings"
)

// Structural validation for generated learner profiles.
//
// A learner profile is not just shown to a parent — its summary is injected
// into every subsequent question-generation prompt for that child, so a
// malformed one degrades what the child is asked next until it is regenerated.
// It was previously stored exactly as the model returned it, with no check at
// all: an empty summary or an empty list entry persisted happily.
//
// Deliberately NOT a content check. Matching against a list of filler words
// ("placeholder", "n/a", …) only catches the tokens we thought of and reads as
// validation without being any. Nor can the schema carry this: minLength and
// minItems are outside the JSON-Schema subset structured outputs enforce, and
// llm.Schema is shared across four providers with no client-side validation
// layer, so declaring them would look like enforcement and do nothing.
//
// What is checkable without guessing at meaning is shape: something was said,
// nothing is blank, nothing ran away. A well-formed but vacuous profile still
// passes — that one needs grounding in real history, not a stricter regex.

// Profile output bounds. The prompt asks for a 3-5 sentence summary and
// entries of 5-10 words; these caps sit well above that, because the aim is to
// catch runaway or empty output, not to police a model that returns five
// strengths instead of four.
const (
	maxProfileSummaryLen = 1500
	maxProfileEntryLen   = 200
	maxProfileEntries    = 10
)

// ErrInvalidProfile means the generated profile was too malformed to store.
// The caller keeps the previous profile rather than overwriting it.
var ErrInvalidProfile = errors.New("generated profile failed validation")

// normalizeProfile trims the model's output. Trailing whitespace reaches the
// parent dashboard verbatim, and blank-after-trim entries are dropped here so
// validation sees the same content a reader would.
func normalizeProfile(out profileOutput) *LearnerProfile {
	return &LearnerProfile{
		Summary:    strings.TrimSpace(out.Summary),
		Strengths:  trimEntries(out.Strengths),
		Weaknesses: trimEntries(out.Weaknesses),
		Patterns:   trimEntries(out.Patterns),
	}
}

func trimEntries(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// validateProfile checks a normalized profile is well formed enough to store.
func validateProfile(p *LearnerProfile) error {
	if p == nil {
		return fmt.Errorf("%w: no profile", ErrInvalidProfile)
	}
	if p.Summary == "" {
		return fmt.Errorf("%w: empty summary", ErrInvalidProfile)
	}
	if len(p.Summary) > maxProfileSummaryLen {
		return fmt.Errorf("%w: summary is %d chars, limit %d", ErrInvalidProfile, len(p.Summary), maxProfileSummaryLen)
	}
	// A profile that names no strength, no weakness and no pattern has told
	// us nothing about the learner, whatever its summary says.
	if len(p.Strengths)+len(p.Weaknesses)+len(p.Patterns) == 0 {
		return fmt.Errorf("%w: no strengths, weaknesses or patterns", ErrInvalidProfile)
	}
	for _, l := range []struct {
		field   string
		entries []string
	}{
		{"strengths", p.Strengths},
		{"weaknesses", p.Weaknesses},
		{"patterns", p.Patterns},
	} {
		if len(l.entries) > maxProfileEntries {
			return fmt.Errorf("%w: %d %s, limit %d", ErrInvalidProfile, len(l.entries), l.field, maxProfileEntries)
		}
		for _, e := range l.entries {
			if len(e) > maxProfileEntryLen {
				return fmt.Errorf("%w: %s entry is %d chars, limit %d", ErrInvalidProfile, l.field, len(e), maxProfileEntryLen)
			}
		}
	}
	return nil
}
