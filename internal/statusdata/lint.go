package statusdata

import (
	"fmt"
	"strings"
)

// Field is one free-text string pulled out of a StatusData, labelled with
// where it came from and word-budgeted the same way render-status would
// budget it: MaxStandingFactWords for a Driver/Constraint, MaxFieldWords
// for everything else. Shared by lint-content (which reports over-budget
// fields at write time) and render-status (which counts them for its own
// diagnostic strip) so the two can't drift apart on what counts as over.
type Field struct {
	Label  string
	Text   string
	Budget int
}

// ExtractFields walks d in the same order render() reads it, pulling out
// only the strings render() actually renders -- a Contribution's second or
// third pushing/constraining item never reaches the report (see
// Contribution's own comment), so a field for it would flag content that's
// already dropped whole, not truncated.
func ExtractFields(d StatusData) []Field {
	var out []Field
	for i, s := range d.Drivers {
		out = append(out, Field{fmt.Sprintf("drivers[%d]", i), s, MaxStandingFactWords})
	}
	for i, s := range d.Constraints {
		out = append(out, Field{fmt.Sprintf("constraints[%d]", i), s, MaxStandingFactWords})
	}
	for i, s := range d.Context {
		out = append(out, Field{fmt.Sprintf("context[%d]", i), s, MaxFieldWords})
	}
	for i, c := range d.Chains {
		if c.Label != "" {
			out = append(out, Field{fmt.Sprintf("chains[%d].label", i), c.Label, MaxFieldWords})
		}
		for j, s := range c.Steps {
			out = append(out, Field{fmt.Sprintf("chains[%d].steps[%d]", i, j), s, MaxFieldWords})
		}
	}
	for i, c := range d.Contributions {
		out = append(out, Field{fmt.Sprintf("contributions[%d].decision", i), c.Decision, MaxFieldWords})
		if len(c.Pushing) > 0 {
			out = append(out, Field{fmt.Sprintf("contributions[%d].pushing[0]", i), c.Pushing[0], MaxFieldWords})
		}
		if len(c.Constraining) > 0 {
			out = append(out, Field{fmt.Sprintf("contributions[%d].constraining[0]", i), c.Constraining[0], MaxFieldWords})
		}
	}
	for i, c := range d.Consequences {
		out = append(out, Field{fmt.Sprintf("consequences[%d].next", i), c.Next, MaxFieldWords})
		out = append(out, Field{fmt.Sprintf("consequences[%d].leads_to", i), c.LeadsTo, MaxFieldWords})
	}
	return out
}

// OverBudget reports a field's word count and whether it exceeds its
// budget -- strings.Fields, not cope's old regex tokenizer, so this counts
// exactly the way render-status's own truncateWords does.
func OverBudget(f Field) (words int, over bool) {
	n := len(strings.Fields(f.Text))
	return n, n > f.Budget
}
