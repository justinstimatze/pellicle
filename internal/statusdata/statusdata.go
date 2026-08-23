// Package statusdata defines status-input.json's schema. Shared between
// render-status (which renders it into status.txt) and lint-content (which
// checks its free-text fields for length before render() ever gets a
// chance to silently truncate one) so the two commands cannot drift apart
// on what a field is called or how it's shaped.
package statusdata

// Contribution replaced an invented for/against confidence ratio
// (2026-08-21) after a 5-agent adversarial pressure-test panel found it
// didn't discriminate: 19 real entries, every one between 7:3 and 9:1,
// none ever reaching even a "close call" gate. The ratio was self-scored
// by the same process, in the same breath, as the decision it described --
// nothing forced it to ever report genuine closeness, so it read as
// confident-sounding decoration rather than a measurement. Pushing/
// Constraining replace it with plain force-field framing (Lewin's driving
// forces vs. restraining forces, not a tttc-style synthetic crux scored
// against a population -- there's no population here, one author) --
// naming a specific factor is harder to fake vacuously than inventing a
// plausible integer pair, and a reader can actually check whether a named
// constraint held up later, the same way a crux was checkable and a bare
// ratio never was.
type Contribution struct {
	Decision string `json:"decision"`
	// Pushing lists the specific factors driving toward this decision --
	// not a single summary clause. Each item should be a fact a later
	// reader could check, not a restatement of the decision itself. Only
	// the first item ever renders (render-status's own maxFactorsPerList);
	// authoring more than one is fine, the rest are dropped whole rather
	// than mangled.
	Pushing []string `json:"pushing"`
	// Constraining lists the specific factors bounding or limiting this
	// decision -- what it traded off against, what makes it fragile.
	// crux used to be a separate field naming the single sharpest such
	// factor (Sabien's "double crux": a fact that, believed differently,
	// changes the conclusion); it's gone as a distinct field, folded in
	// here as an ordinary constraining item written with that same
	// specificity, not a separate schema concept.
	Constraining []string `json:"constraining"`
	Durability   string   `json:"durability"` // "provisional" or "durable"
}

// Consequence is a next/leads_to pair -- one forward hop, not a sequence
// (see Chain for that).
type Consequence struct {
	Next    string `json:"next"`
	LeadsTo string `json:"leads_to"`
	// Risky flags a next action as high-stakes or hard to reverse (a
	// force-push, a destructive migration, a public post) -- rendered
	// distinctly from a routine next step (⚠ instead of ☐), so the glyph
	// alone carries the signal for a reader glancing at the report.
	Risky bool `json:"risky,omitempty"`
	// RiskyCommand is a literal substring (not a regex, not free prose) of
	// the one specific shell command this risky Consequence maps to -- e.g.
	// "push --force" or "DROP TABLE". Operational metadata, not
	// reader-facing content: render() never displays it. status-risk-gate.sh
	// (a PreToolUse hook, not render-status) reads it straight from
	// status-input.json and escalates a matching Bash call to a permission
	// prompt carrying this Consequence's own next/leads_to as the reason.
	// Meaningful only alongside Risky==true; nothing here enforces the
	// pairing -- risky_command without risky: true is meaningless (the gate
	// hook checks Risky first), and risky: true without risky_command is
	// the normal case (most risky actions aren't reducible to one matchable
	// command, and stay purely decorative -- the ⚠ glyph).
	RiskyCommand string `json:"risky_command,omitempty"`
}

// Chain renders a causal sequence -- A led to B led to C. Distinct from
// Consequence's next/leads_to, which is always exactly one hop.
// MinChainSteps is lucida's own bar for this shape: orchestrator.py's
// mermaid trivial-filter also requires >=3 nodes before it earns a diagram
// over prose ("too few nodes -- a 1-2 node graph reads as prose").
type Chain struct {
	Label string   `json:"label"` // optional, 1-4 words
	Steps []string `json:"steps"`
}

// MinChainSteps is the minimum step count render() requires -- fewer than
// this and render() errors rather than silently demoting the chain to
// prose.
const MinChainSteps = 3

// MaxFieldWords is the word budget render-status truncates a single
// clause-shaped field to (a Context line, a Contribution's decision or its
// one kept pushing/constraining item, a Consequence's next/leads_to, a
// chain label or step). lint-content checks the same budget before
// render() ever gets a chance to silently truncate one, rather than the
// two tools drifting apart on the number. See render-status's own
// maxLineWords comment for how it was calibrated.
const MaxFieldWords = 20

// MaxStandingFactWords is the shorter budget for a Driver/Constraint item.
// A standing fact is meant to read like "ship by EOD", not a full clause.
// See render-status's own maxStandingFactorWords comment.
const MaxStandingFactWords = 12

// StatusData is status-input.json's top-level shape.
type StatusData struct {
	// Drivers/Constraints are standing, project-level facts -- "ship by
	// EOD", "can't use this library, its license blocks us" -- not tied
	// to any one decision the way a Contribution's Pushing/Constraining
	// are. Added (2026-08-21) after Contributions' pushing/constraining
	// turned out too granular for what this section actually needs to
	// surface: a real constraint like a license block shouldn't live and die with
	// whichever Contribution happened to first mention it and then age
	// out under render-status's own cap -- it's still true and still
	// binding long after that. Both optional and both sparse by design:
	// most turns have nothing NEW to say here, and forcing authorship when
	// nothing standing has changed is exactly the padding this project has
	// spent effort cutting, not adding.
	Drivers       []string       `json:"drivers,omitempty"`
	Constraints   []string       `json:"constraints,omitempty"`
	Context       []string       `json:"context"`
	Chains        []Chain        `json:"chains"`
	Contributions []Contribution `json:"contributions"`
	Consequences  []Consequence  `json:"consequences"`
}
