package statusdata

import (
	"strings"
	"testing"
)

func TestExtractFieldsWalksEverySection(t *testing.T) {
	d := StatusData{
		Drivers:     []string{"ship by EOD"},
		Constraints: []string{"license blocks this library"},
		Context:     []string{"a settled fact"},
		Chains: []Chain{
			{Label: "a label", Steps: []string{"step one", "step two", "step three"}},
		},
		Contributions: []Contribution{
			{Decision: "a decision", Pushing: []string{"p"}, Constraining: []string{"c"}, Durability: "durable"},
		},
		Consequences: []Consequence{
			{Next: "the next action", LeadsTo: "where it leads"},
		},
	}
	fs := ExtractFields(d)

	want := map[string]string{
		"drivers[0]":                       "ship by EOD",
		"constraints[0]":                   "license blocks this library",
		"context[0]":                       "a settled fact",
		"chains[0].label":                  "a label",
		"chains[0].steps[0]":               "step one",
		"chains[0].steps[1]":               "step two",
		"chains[0].steps[2]":               "step three",
		"contributions[0].decision":        "a decision",
		"contributions[0].pushing[0]":      "p",
		"contributions[0].constraining[0]": "c",
		"consequences[0].next":             "the next action",
		"consequences[0].leads_to":         "where it leads",
	}
	if len(fs) != len(want) {
		t.Fatalf("got %d fields, want %d: %+v", len(fs), len(want), fs)
	}
	for _, f := range fs {
		if want[f.Label] != f.Text {
			t.Errorf("field %q = %q, want %q", f.Label, f.Text, want[f.Label])
		}
	}
}

func TestExtractFieldsUsesTheShorterBudgetForDriversAndConstraintsOnly(t *testing.T) {
	d := StatusData{
		Drivers:     []string{"a driver"},
		Constraints: []string{"a constraint"},
		Context:     []string{"a context line"},
		Contributions: []Contribution{
			{Decision: "d", Pushing: []string{"p"}, Constraining: []string{"c"}, Durability: "durable"},
		},
		Consequences: []Consequence{{Next: "n", LeadsTo: "l"}},
	}
	for _, f := range ExtractFields(d) {
		wantShort := f.Label == "drivers[0]" || f.Label == "constraints[0]"
		if wantShort && f.Budget != MaxStandingFactWords {
			t.Errorf("%s: got budget %d, want the shorter MaxStandingFactWords (%d)", f.Label, f.Budget, MaxStandingFactWords)
		}
		if !wantShort && f.Budget != MaxFieldWords {
			t.Errorf("%s: got budget %d, want MaxFieldWords (%d)", f.Label, f.Budget, MaxFieldWords)
		}
	}
}

func TestExtractFieldsOnlyKeepsTheFirstPushingAndConstrainingItem(t *testing.T) {
	// A second/third pushing/constraining item never reaches render()'s
	// output (maxFactorsPerList keeps only the first) -- linting it would
	// flag content that's already dropped whole, not truncated, which
	// isn't the bug this tool exists to catch.
	d := StatusData{
		Contributions: []Contribution{
			{Decision: "d", Pushing: []string{"first", "second"}, Constraining: []string{"first c", "second c"}, Durability: "durable"},
		},
	}
	for _, f := range ExtractFields(d) {
		if strings.Contains(f.Text, "second") {
			t.Errorf("field %q = %q, a dropped second item should never be checked", f.Label, f.Text)
		}
	}
}

func TestExtractFieldsSkipsEmptyChainLabel(t *testing.T) {
	d := StatusData{
		Chains: []Chain{{Steps: []string{"a", "b", "c"}}},
	}
	for _, f := range ExtractFields(d) {
		if f.Label == "chains[0].label" {
			t.Error("an empty chain label should not produce a field to check")
		}
	}
}

func TestOverBudgetCountsPlainWhitespaceWordsNotHyphenSplitOnes(t *testing.T) {
	// The whole point of doing this natively: "cope-gate" and
	// "status-risk-gate.sh" must count as ONE word each, matching
	// render()'s own strings.Fields-based truncateWords, not cope's old
	// [A-Za-z']+ regex tokenizer (which split every hyphen and period).
	f := Field{Label: "x", Text: "a real second binary dependency (cope-gate) on PATH", Budget: 20}
	words, over := OverBudget(f)
	if words != 8 {
		t.Errorf("got %d words, want 8 (plain whitespace split)", words)
	}
	if over {
		t.Errorf("8 words under a budget of 20 should not be over")
	}
}

func TestOverBudgetFlagsAGenuineOutlier(t *testing.T) {
	f := Field{Label: "x", Text: strings.Repeat("word ", 30) + "end", Budget: 20}
	words, over := OverBudget(f)
	if words != 31 {
		t.Errorf("got %d words, want 31", words)
	}
	if !over {
		t.Error("31 words over a budget of 20 should be over")
	}
}

func TestOverBudgetAtExactlyTheBudgetIsNotOver(t *testing.T) {
	f := Field{Label: "x", Text: strings.Repeat("word ", 19) + "end", Budget: 20}
	words, over := OverBudget(f)
	if words != 20 {
		t.Errorf("got %d words, want 20", words)
	}
	if over {
		t.Error("exactly 20 words against a budget of 20 should not be over -- render()'s own cap is words <= 20, not words < 20")
	}
}
