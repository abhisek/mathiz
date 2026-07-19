package problemgen

import "testing"

// ShuffleChoices must preserve the choice SET and the answer text (checking
// is text-based), while actually randomizing order — an unshuffled deck
// teaches kids "pick A" because LLMs put the correct answer first.
func TestShuffleChoices(t *testing.T) {
	orig := []string{"612", "623", "633", "652"}

	sawDifferentOrder := false
	for i := 0; i < 60; i++ {
		q := &Question{
			Format:  FormatMultipleChoice,
			Answer:  "623",
			Choices: append([]string(nil), orig...),
		}
		q.ShuffleChoices()

		if q.Answer != "623" {
			t.Fatalf("answer text changed: %q", q.Answer)
		}
		seen := make(map[string]bool, len(q.Choices))
		for _, c := range q.Choices {
			seen[c] = true
		}
		for _, c := range orig {
			if !seen[c] {
				t.Fatalf("choice %q lost in shuffle: %v", c, q.Choices)
			}
		}
		for j, c := range q.Choices {
			if c != orig[j] {
				sawDifferentOrder = true
				break
			}
		}
	}
	// P(60 identity shuffles of 4 items) = (1/24)^60 — effectively zero.
	if !sawDifferentOrder {
		t.Error("60 shuffles never changed the order — shuffle is a no-op")
	}
}

// Non-MCQ questions and degenerate choice lists must be left alone.
func TestShuffleChoicesNoOpCases(t *testing.T) {
	numeric := &Question{Format: FormatNumeric, Answer: "42"}
	numeric.ShuffleChoices()
	if numeric.Choices != nil {
		t.Errorf("numeric question grew choices: %v", numeric.Choices)
	}

	single := &Question{Format: FormatMultipleChoice, Answer: "x", Choices: []string{"x"}}
	single.ShuffleChoices()
	if len(single.Choices) != 1 || single.Choices[0] != "x" {
		t.Errorf("single-choice list mutated: %v", single.Choices)
	}
}

// Position-dependent options must end up pinned at the end after a shuffle,
// or "all of the above" can land mid-list and corrupt the question.
func TestShuffleChoicesPinsPositionDependentOptions(t *testing.T) {
	for i := 0; i < 40; i++ {
		q := &Question{
			Format:  FormatMultipleChoice,
			Answer:  "All of the above",
			Choices: []string{"All of the above", "12", "15", "18"},
		}
		q.ShuffleChoices()
		if got := q.Choices[len(q.Choices)-1]; got != "All of the above" {
			t.Fatalf("run %d: position-dependent option not pinned last: %v", i, q.Choices)
		}
	}

	// Two pinned options keep a stable tail; normal options still present.
	q := &Question{
		Format:  FormatMultipleChoice,
		Answer:  "7",
		Choices: []string{"None of these", "7", "All of the above", "9"},
	}
	q.ShuffleChoices()
	n := len(q.Choices)
	tail := map[string]bool{q.Choices[n-2]: true, q.Choices[n-1]: true}
	if !tail["None of these"] || !tail["All of the above"] {
		t.Fatalf("pinned options not at the tail: %v", q.Choices)
	}
}
