package lessons

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/abhisek/mathiz/internal/llm"
)

func TestValidateProfile(t *testing.T) {
	valid := profileOutput{
		Summary:    "Sheoli is solid on addition and is working through regrouping.",
		Strengths:  []string{"solid addition facts"},
		Weaknesses: []string{"regrouping in subtraction"},
		Patterns:   []string{"rushes on timed questions"},
	}

	tests := []struct {
		name    string
		out     profileOutput
		wantErr bool
	}{
		{name: "valid", out: valid},
		{
			name: "summary only, no lists",
			out:  profileOutput{Summary: "Doing fine."},
			// Nothing was actually said about the learner.
			wantErr: true,
		},
		{
			name:    "empty summary",
			out:     profileOutput{Strengths: []string{"addition"}},
			wantErr: true,
		},
		{
			name:    "whitespace-only summary",
			out:     profileOutput{Summary: "   \n\t ", Strengths: []string{"addition"}},
			wantErr: true,
		},
		{
			name: "blank list entries only",
			out: profileOutput{
				Summary:   "Doing fine.",
				Strengths: []string{"", "   "},
			},
			wantErr: true,
		},
		{
			name: "runaway summary",
			out: profileOutput{
				Summary:   strings.Repeat("x", maxProfileSummaryLen+1),
				Strengths: []string{"addition"},
			},
			wantErr: true,
		},
		{
			name: "runaway entry",
			out: profileOutput{
				Summary:   "Doing fine.",
				Strengths: []string{strings.Repeat("x", maxProfileEntryLen+1)},
			},
			wantErr: true,
		},
		{
			name: "too many entries",
			out: profileOutput{
				Summary:   "Doing fine.",
				Strengths: make([]string, 0),
				Patterns:  fill(maxProfileEntries + 1),
			},
			wantErr: true,
		},
		{
			name: "slightly off-spec count is accepted",
			out: profileOutput{
				Summary:   "Doing fine.",
				Strengths: fill(5), // prompt asks for 2-4; not worth rejecting
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateProfile(normalizeProfile(tc.out))
			if tc.wantErr && err == nil {
				t.Fatal("want validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want valid, got %v", err)
			}
			if tc.wantErr && !errors.Is(err, ErrInvalidProfile) {
				t.Errorf("err = %v, want ErrInvalidProfile", err)
			}
		})
	}
}

func TestNormalizeProfileTrims(t *testing.T) {
	p := normalizeProfile(profileOutput{
		Summary:   "  Doing fine.\n",
		Strengths: []string{"  addition  ", "", "   "},
	})
	if p.Summary != "Doing fine." {
		t.Errorf("summary = %q, want trimmed", p.Summary)
	}
	if len(p.Strengths) != 1 || p.Strengths[0] != "addition" {
		t.Errorf("strengths = %q, want [addition]", p.Strengths)
	}
}

// TestGenerateProfileRejectsMalformedOutput checks the validation is actually
// wired into the generation path, not just available beside it. An empty
// summary is schema-valid — ProfileSchema cannot express minLength — so
// without this check it would be stored and rendered to the parent verbatim.
func TestGenerateProfileRejectsMalformedOutput(t *testing.T) {
	provider := llm.NewMockProvider(llm.MockResponse{
		Content: []byte(`{"summary":"","strengths":[],"weaknesses":[],"patterns":[]}`),
	})
	c := NewCompressor(provider, DefaultCompressorConfig())

	profile, err := c.GenerateProfile(context.Background(), ProfileInput{})
	if !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("err = %v, want ErrInvalidProfile", err)
	}
	if profile != nil {
		t.Errorf("profile = %+v, want nil so the caller keeps the previous one", profile)
	}
}

func fill(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "entry"
	}
	return out
}
