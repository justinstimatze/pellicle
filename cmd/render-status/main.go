// render-status renders pellicle's status.txt from structured data instead
// of a hand-edited generator script. Input is a JSON document (file arg or
// stdin) describing Context prose, Contributions (specific decisions with a
// bias ratio and durability marker), and Consequences (next/leads-to pairs).
// The three sections are told apart by shape alone -- no printed headers.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/justinstimatze/pellicle/internal/statusdata"
)

// version is "dev" by default and baked at release time via
//
//	go install -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/render-status
//
// The git tag is the single source of truth — there is no hand-maintained
// version constant to drift out of sync. buildVersion() resolves it.
var version = "dev"

// buildVersion reports the binary's version, preferring (in order): a
// release value baked in via -ldflags; the module version when installed
// with `go install …@vX.Y.Z`; the embedded VCS commit (+dirty) for local
// `go build`. Falls back to "dev" when none is available (e.g. a tarball
// build outside a git tree).
func buildVersion() string {
	if version != "dev" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 12 {
				rev = s.Value[:12]
			} else {
				rev = s.Value
			}
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev != "" {
		return rev + dirty
	}
	return version
}

const (
	// maxContextLines caps Context -- a safety valve, not content policy.
	// Silently keeps the newest N rather than appending a "N more..." line:
	// matches lucida (github.com/justinstimatze/lucida) orchestrator.py's
	// "silent > text" rule (SuppressedMintError) -- a visible truncation
	// marker is itself low-value noise in a status report.
	// Lowered 12 -> 6 (2026-08-21): a peripheral display should hold fewer
	// things at once, not more -- see Weiser & Brown, "The Coming Age of
	// Calm Technology" (calmtech.com/papers/coming-age-calm-technology):
	// "by placing things in the periphery we are able to attune to many
	// more things than we could if everything had to be at the center...
	// the periphery is informing without overburdening." Twelve settled
	// facts is a list to read, not a glance.
	maxContextLines = 6

	// maxContributions is the same cap, for the same reason, on the section
	// Context's own cap didn't cover: Contributions once grew to 7 entries
	// (14 lines) uncapped and pushed a 37-line render past the raw-tty
	// mechanism's own live safety cap (that mechanism is gone now, see
	// CHANGELOG -- the report-length reasoning below still holds on its
	// own). Two lines per entry -- keep this small. Lowered 4 -> 3
	// alongside maxContextLines, same fewer-things-in-periphery reasoning.
	maxContributions = 3

	// maxConsequences closes the same gap for the one remaining unbounded
	// section -- found (2026-08-21) when a full report legitimately hit 34
	// lines because Consequences had no cap at all, same shape as
	// maxContributions' own overflow above, fixed the same way. Lowered
	// 4 -> 2: Consequences is "what's next," and more than a couple of live
	// threads at once is already more than a glance can hold.
	maxConsequences = 2

	// maxLineWords truncates any single free-text field (a Context line, a
	// Contribution's decision/tradeoff/crux, a Consequence's
	// next/leads_to, a chain step) as a safety valve against a genuine
	// outlier, not routine trimming. Was 10, calibrated to a "dataword"
	// ideal (Tufte's sparkline definition: "data-intense, design-simple,
	// word-sized graphics") that didn't match how these fields actually
	// get written -- measured real content in this project's own
	// status-input.json (2026-08-21) and most lines run 13-23 words in a
	// "clause -- payload" shape, so a 10-word cut landed mid-second-clause
	// on nearly every line, cutting exactly the part carrying the point
	// (worst case: a crux field cut right before its own conditional
	// clause -- the entire reason the field exists). Raised to 20, which
	// covers that real range and leaves only genuine outliers (few real
	// lines run past it) truncated. The number itself lives in
	// internal/statusdata (MaxFieldWords) so lint-content checks the exact
	// same budget before render() ever gets a chance to silently truncate
	// a field, rather than the two tools drifting apart on it.
	maxLineWords = statusdata.MaxFieldWords

	// tallyReserveChars is subtracted from maxLineChars when truncating a
	// Consequence's "next" text that will carry a tool-tally suffix (e.g.
	// "  — 9999 calls, 999m") -- see truncateWordsCharCap's own comment
	// for why this is a character reservation, not a word-count one. 24
	// comfortably covers the tally's own worst plausible length (measured
	// at 18-21 chars for realistic call counts and durations) with margin
	// to spare.
	tallyReserveChars = 24

	// minSparkPoints/maxSparkPoints follow lucida's own sparkline specialist
	// rule (specialists.py SPARKLINE_SYSTEM: "6-40 points... below 6 points,
	// callouts read better") -- not lucida's actual code gate, which drifted
	// to <4 (orchestrator.py _trivial_sparkline) and contradicts its own
	// message text. The design intent is 6, so pellicle uses 6.
	minSparkPoints = 6
	maxSparkPoints = 40

	// maxChains closes the one section every other cap here already covers
	// -- Chains had no cap at all, so a single chain, once added, rendered
	// unconditionally forever regardless of relevance (caught live: the
	// row-1-debugging chain from this project's early history was still
	// rendering every turn, months of real content later). Keep-newest, 1
	// -- a chain is already a 4+ line structure (label + 3+ steps), so more
	// than one at a time would dominate the whole peripheral display, and
	// chains are meant to be rare (see statusdata.MinChainSteps' own comment: "too
	// few nodes reads as prose").
	maxChains = 1

	// maxFactorsPerList caps how many pushing/constraining items render per
	// Contribution -- silent-keep-first, same shape as every other cap
	// here. Dropped 3 -> 1 (2026-08-22): measured against this project's
	// own real status-input.json, pushing/constraining items run a 17-word
	// median and 28-word max -- already most of a single line's own
	// maxLineWords budget on their own. Joining up to 3 of those onto one
	// line and truncating the JOINED blob to that same 20-word budget was
	// silently destroying real content mid-sentence, not just shortening
	// it: a genuinely load-bearing second factor ("still self-reported by
	// the same process... doesn't add an independent check") got cut to
	// three words and vanished. One factor, given the full line to itself,
	// is a real fact a reader can check; a second or third factor is
	// dropped whole rather than half-rendered -- worth revisiting (2
	// factors, each its own line) if one factor per direction turns out
	// too thin in practice.
	maxFactorsPerList = 1

	// maxStandingFactors caps Drivers and Constraints (statusdata.StatusData's own
	// comment) at the same size, keep-newest. The design brief for this
	// section was "the salient ones" -- a list past a handful stops being
	// salient and starts being everything.
	maxStandingFactors = 4

	// maxStandingFactorWords is the per-item truncation budget joinTruncated
	// applies to each Driver/Constraint before joining them onto one line --
	// deliberately smaller than maxLineWords. Drivers/Constraints are
	// standing facts by their own definition ("ship by EOD", "can't use
	// this library, its license blocks us"), not full clauses the way a
	// Contribution's decision or a Context line are, so a shorter per-item
	// budget matches what the section is actually for. No real content has
	// exercised this yet (Drivers/Constraints are sparse by design -- see
	// statusdata.StatusData's own comment); chosen to comfortably fit the
	// example facts above ("ship by EOD", "can't use this library, its
	// license blocks us") rather than measured against real overflow. The number
	// itself lives in internal/statusdata (MaxStandingFactWords), same
	// reasoning as maxLineWords above.
	maxStandingFactorWords = statusdata.MaxStandingFactWords
)

// statusGlyph gives durability a shape -- hollow for provisional, filled
// for durable, the same open/settled encoding this report already uses
// for ☐/☑ on Consequences, rather than an arbitrary distinct icon.
func statusGlyph(durability string) string {
	if durability == "provisional" {
		return "○"
	}
	return "●"
}

// contribution, consequence, chain, and statusData moved to
// internal/statusdata (2026-08-22) so lint-content can parse the same
// schema render-status does without the two drifting apart on what a
// field is called or how it's shaped.

// sanitizeText strips C0 control characters and DEL from free text before it
// reaches status.txt. status-input.json is Claude-authored per turn and can
// echo text read from elsewhere in the conversation (a file, a fetched
// page) -- an embedded control byte would otherwise survive untouched into
// a file whose one-row-per-line layout it can break, and into the Stop
// hook's systemMessage JSON, which status-fallback.sh builds from this
// file's raw bytes (jq escapes them safely either way, but there's no
// reason to carry a stray control character into a transcript at all).
func sanitizeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// sanitizeStatusData applies sanitizeText to every free-text field render()
// ever prints. Applied once, right after JSON decoding, so render() can
// trust the strings it holds without re-sanitizing at each call site.
// RiskyCommand is deliberately not
// included here -- render() never displays it (it's read straight out of
// status-input.json by status-risk-gate.sh, not through this binary at
// all), so sanitizing it here wouldn't reach the place it actually matters.
func sanitizeStatusData(d statusdata.StatusData) statusdata.StatusData {
	for i := range d.Drivers {
		d.Drivers[i] = sanitizeText(d.Drivers[i])
	}
	for i := range d.Constraints {
		d.Constraints[i] = sanitizeText(d.Constraints[i])
	}
	for i := range d.Context {
		d.Context[i] = sanitizeText(d.Context[i])
	}
	for i := range d.Chains {
		d.Chains[i].Label = sanitizeText(d.Chains[i].Label)
		for j := range d.Chains[i].Steps {
			d.Chains[i].Steps[j] = sanitizeText(d.Chains[i].Steps[j])
		}
	}
	for i := range d.Contributions {
		d.Contributions[i].Decision = sanitizeText(d.Contributions[i].Decision)
		for j := range d.Contributions[i].Pushing {
			d.Contributions[i].Pushing[j] = sanitizeText(d.Contributions[i].Pushing[j])
		}
		for j := range d.Contributions[i].Constraining {
			d.Contributions[i].Constraining[j] = sanitizeText(d.Contributions[i].Constraining[j])
		}
	}
	for i := range d.Consequences {
		d.Consequences[i].Next = sanitizeText(d.Consequences[i].Next)
		d.Consequences[i].LeadsTo = sanitizeText(d.Consequences[i].LeadsTo)
	}
	return d
}

// capStatusData prunes d down to exactly the keep-newest slices render()
// already caps each section to on its own (maxContextLines,
// maxContributions, maxConsequences, maxChains, maxStandingFactors) --
// duplicated as a no-op here, not a new truncation rule: those caps only
// ever trimmed what got displayed, while status-input.json itself kept
// every entry ever written, forever. That gap is why lint-content's
// over-budget count only ever climbed turn over turn: most of what it
// flagged hadn't been shown in status.txt for dozens of turns already, not
// because it was too long, but because it had aged out of every section's
// display cap without ever being removed from the file. Called once in
// main(), right after sanitizeStatusData, so main() can write the capped d
// straight back to status-input.json -- see its own comment for why only
// a real file input (not stdin) gets pruned.
func capStatusData(d statusdata.StatusData) statusdata.StatusData {
	if len(d.Context) > maxContextLines {
		d.Context = d.Context[len(d.Context)-maxContextLines:]
	}
	if len(d.Contributions) > maxContributions {
		d.Contributions = d.Contributions[len(d.Contributions)-maxContributions:]
	}
	if len(d.Consequences) > maxConsequences {
		d.Consequences = d.Consequences[len(d.Consequences)-maxConsequences:]
	}
	if len(d.Chains) > maxChains {
		d.Chains = d.Chains[len(d.Chains)-maxChains:]
	}
	if len(d.Drivers) > maxStandingFactors {
		d.Drivers = d.Drivers[len(d.Drivers)-maxStandingFactors:]
	}
	if len(d.Constraints) > maxStandingFactors {
		d.Constraints = d.Constraints[len(d.Constraints)-maxStandingFactors:]
	}
	return d
}

// marshalPrunedInput serializes d back into status-input.json's own shape,
// for main()'s prune-on-render write-back. Uses an Encoder with
// SetEscapeHTML(false) rather than plain json.Marshal/MarshalIndent --
// Go's json package HTML-escapes <, >, and & inside string values by
// default (it assumes web-embedded output unless told otherwise), which
// silently mangled real content the first time this ran live: every
// literal "-" followed by ">" became the six characters backslash-u-0-0-
// 3-e, and every literal "<" became backslash-u-0-0-3-c. Still valid
// JSON either way, but status-input.json is meant to stay directly
// human- and grep-readable across every future hand edit, not just
// machine-parseable.
func marshalPrunedInput(d statusdata.StatusData) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(d); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// provisionalRatio is the fraction of Contributions marked "provisional"
// in a render call -- replaced meanBiasRatio (2026-08-21), which averaged
// each Contribution's own for/against self-score into a "bias" number that
// answered the wrong question: it tracked how confident the scoring was,
// not anything about the project. A session where this trends toward 0
// (everything durable) is a real signal worth a glance -- confidence
// hasn't just gone up, hedging has stopped, which is exactly the kind of
// drift a reader re-entering the session should be able to catch. A
// per-decision series was considered instead of per-render, but decisions
// are specific and turn-scoped -- exact wording rarely repeats, so a
// per-decision series would almost never reach minSparkPoints.
func provisionalRatio(cs []statusdata.Contribution) (float64, bool) {
	if len(cs) == 0 {
		return 0, false
	}
	var provisional int
	for _, c := range cs {
		if c.Durability == "provisional" {
			provisional++
		}
	}
	return float64(provisional) / float64(len(cs)), true
}

// historyPoint's two metrics are each independently optional: Ratio needs
// at least one Contribution (provisionalRatio's own gate), Calls needs at
// least one Consequence with a live tool tally, and a real render can have
// either, both, or neither. Pointers, not a bare float64/int plus a
// separate "present" bool -- nil already means "not recorded" without a
// second field that could drift out of sync with the first.
type historyPoint struct {
	Time  int64    `json:"time"`
	Ratio *float64 `json:"ratio,omitempty"`
	Calls *int     `json:"calls,omitempty"`
}

// historyPath derives status-history.jsonl as a sibling of out, the same
// convention paint.log uses in main.go (derived from content_file, not a
// hardcoded path) -- each project adopting pellicle gets its own history.
func historyPath(out string) string {
	return filepath.Join(filepath.Dir(out), "status-history.jsonl")
}

// appendHistory is best-effort: a failed write here must never fail the
// render it's attached to, matching logPaint's own "never fail loudly" rule
// in main.go. ratio and calls are each nil when this render has nothing to
// say about that metric; a point with neither is skipped entirely, since
// there's nothing in it to ever read back out.
func appendHistory(path string, ratio *float64, calls *int) error {
	if ratio == nil && calls == nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(historyPoint{Time: time.Now().Unix(), Ratio: ratio, Calls: calls})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, string(line))
	return err
}

// readHistory returns every point in path, oldest first, both series still
// interleaved and uncapped -- ratioSeries/callsSeries below extract and cap
// each one independently, since the two are gated on different content and
// a real render's history can have gaps in either one that don't line up.
// A malformed line is skipped, not fatal -- consistent with the rest of
// this file treating the history/log files as best-effort, not a source of
// truth.
func readHistory(path string) ([]historyPoint, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	var points []historyPoint
	for _, l := range lines {
		if l == "" {
			continue
		}
		var p historyPoint
		if err := json.Unmarshal([]byte(l), &p); err != nil {
			continue
		}
		points = append(points, p)
	}
	return points, nil
}

// lastN keeps the newest n values, same keep-newest shape as every other
// cap in this file.
func lastN(vals []float64, n int) []float64 {
	if len(vals) > n {
		return vals[len(vals)-n:]
	}
	return vals
}

// ratioSeries/callsSeries pull one metric out of points, skipping any point
// where it wasn't recorded, then cap to the newest maxSparkPoints -- capped
// per series rather than per point, since ratioSeries and callsSeries can
// have gaps in different places.
func ratioSeries(points []historyPoint) []float64 {
	var out []float64
	for _, p := range points {
		if p.Ratio != nil {
			out = append(out, *p.Ratio)
		}
	}
	return lastN(out, maxSparkPoints)
}

func callsSeries(points []historyPoint) []float64 {
	var out []float64
	for _, p := range points {
		if p.Calls != nil {
			out = append(out, float64(*p.Calls))
		}
	}
	return lastN(out, maxSparkPoints)
}

// toolCount is the sidecar status-tool-count.sh increments on every tool
// call and render-status resets whenever it sees a new goal -- the
// objective, externally-computed proportionality signal this feature
// exists to provide. The failure mode it's for: rabbit holes
// (disproportionate effort on the current goal, unnoticed) don't have a
// mechanism the way a misjudged tradeoff already
// does (Contributions' for/against ratio, crux, closest-call callout).
// Self-report doesn't work for a rabbit hole specifically -- the same
// compromised judgment that causes one also compromises noticing it in
// the moment -- so this started as a plain externally-computed fact next
// to the goal it's about, not a self-assessment: no threshold, no alert.
//
// expireStaleConsequence (2026-08-22) adds a threshold after all, but not
// the kind the paragraph above rules out: it drops content from the
// report, an automated display decision render() makes on its own, not an
// alert asking Claude to notice and judge its own rabbit hole mid-session.
// The self-report failure mode was specifically about the LATTER --
// compromised judgment can't reliably notice itself -- and this doesn't
// ask anyone to notice anything.
type toolCount struct {
	Goal  string `json:"goal"`
	Count int    `json:"count"`
	Since int64  `json:"since"`
}

// expireAfterCalls is a first guess, not a measured calibration -- unlike
// maxLineWords (calibrated against this project's own real content before
// picking 20 over 10), there's no real distribution of "how many tool
// calls does a goal legitimately take" to measure yet. 25 is picked to be
// clearly past a normal goal's own lifespan without being trigger-happy;
// tune it the same way this file's other constants got tuned, once real
// dogfooding shows it firing too early or too late.
const expireAfterCalls = 25

// expireStaleConsequence drops the newest Consequence once tc's own tally
// crosses expireAfterCalls -- the same clock the tally already displays,
// now also deciding when a goal has sat long enough to stop being shown as
// current. tc.Goal must match the newest Consequence's own Next for the
// count to mean anything: by the time this is called (after
// loadToolTally), tc.Goal always matches -- loadToolTally resets the
// count to zero the moment the goal itself changes, so a stale count can
// never be attributed to a different, newer goal. The check is kept
// explicit anyway as the invariant this function actually depends on, not
// as dead defensive code.
func expireStaleConsequence(d statusdata.StatusData, tc toolCount) statusdata.StatusData {
	if len(d.Consequences) == 0 {
		return d
	}
	goal := d.Consequences[len(d.Consequences)-1].Next
	if tc.Goal != goal || tc.Count <= expireAfterCalls {
		return d
	}
	d.Consequences = d.Consequences[:len(d.Consequences)-1]
	return d
}

// toolCountFile is cwd-relative -- NOT a sibling of -out the way
// historyPath derives status-history.jsonl from it. render-status is
// always invoked from the project root by every existing convention in
// this codebase (status-input.json and status.txt are both bare relative
// names already), so this isn't a new assumption -- and it has to be this
// exact literal path, since it's also the path status-tool-count.sh
// itself reads and writes.
const toolCountFile = ".pellicle-tool-count.json"

// readToolCount reads path's tool-count sidecar. A missing or malformed
// file both return the zero value (empty Goal), which syncToolCount below
// treats identically to "no prior goal ever recorded" -- this doubles as
// the recovery path for a count file status-tool-count.sh's own
// best-effort writes left corrupted: readToolCount here won't repair it,
// but syncToolCount will, the next time a real render sees a "new" goal
// against an empty one and overwrites the file.
func readToolCount(path string) toolCount {
	raw, err := os.ReadFile(path)
	if err != nil {
		return toolCount{}
	}
	var tc toolCount
	if err := json.Unmarshal(raw, &tc); err != nil {
		return toolCount{}
	}
	return tc
}

// writeToolCount persists a reset tally -- the one write render-status
// itself ever performs on this file; every other write is status-tool-
// count.sh's own per-tool-call increment. Not atomic (no
// temp-file-then-rename): the same tradeoff status-tool-count.sh's own
// comment names for its increments applies here too -- an occasional lost
// update is an acceptable cost for a proportionality signal, not an exact
// ledger, and the two writers (this function, the hook script) racing
// each other is the same already-accepted risk, not a new one.
func writeToolCount(path string, tc toolCount) error {
	raw, err := json.Marshal(tc)
	if err != nil {
		return err
	}
	return atomicWriteFile(path, raw, 0o644)
}

// atomicWriteFile writes data to path via a temp file in the same directory
// plus a rename, so no reader (status-tool-count.sh's own read-modify-write,
// or a concurrent render-status invocation) ever observes a partial write
// -- two racing
// plain writes to the same path can otherwise interleave into truncated,
// invalid JSON, not just a lost update. os.Rename also doesn't follow a
// symlink at its destination, so a pre-planted symlink at path gets
// replaced outright instead of having its target truncated.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		os.Remove(tmpPath)
		return writeErr
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return closeErr
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// syncToolCount decides whether goal (the current render's
// most-recently-authored Consequence -- Consequences[len-1].Next) is the
// same goal cur was already tracking.
// A differing goal -- including cur being the zero value, from a missing
// or malformed sidecar file -- resets the tally to zero: counting forward
// from whatever was there before would attribute a prior goal's effort to
// a new one, which is exactly the proportionality judgment this feature
// exists to keep honest. A matching goal returns cur completely
// unchanged, for the caller to display as-is -- incrementing count is
// status-tool-count.sh's job on the next tool call, not render's.
func syncToolCount(cur toolCount, goal string, now int64) (toolCount, bool) {
	if cur.Goal == goal {
		return cur, false
	}
	return toolCount{Goal: goal, Count: 0, Since: now}, true
}

// loadToolTally reads path, decides via syncToolCount whether the goal
// changed since the last render, writes the reset back when it did (this
// is the one place besides the hook script that ever writes this file),
// and returns the toolCount this render should display either way. A
// write failure is reported to the caller but never fails the render it's
// attached to -- same best-effort contract as appendHistory.
func loadToolTally(path, goal string, now int64) (toolCount, error) {
	cur := readToolCount(path)
	tc, changed := syncToolCount(cur, goal, now)
	if !changed {
		return tc, nil
	}
	return tc, writeToolCount(path, tc)
}

// formatDuration renders elapsed seconds in the compact form this
// peripheral display calls for -- seconds below a minute (a just-reset
// goal should read "3s", not "0m", which would look frozen or broken),
// whole minutes at and above. No hours unit and no fractional minutes:
// this is a proportionality glance, not a stopwatch.
func formatDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm", seconds/60)
}

// formatToolTally renders the proportionality signal appended to the
// currently-active goal's own "next" line: raw tool-call count and
// elapsed wall time since that goal was set, nothing else -- no
// threshold, no verdict. Matches this project's own already-reasoned-
// through design choice to avoid self-assessment (see toolCount's own
// comment): a human judges proportionality, this just states the fact.
func formatToolTally(count int, elapsedSeconds int64) string {
	call := "calls"
	if count == 1 {
		call = "call"
	}
	return fmt.Sprintf("  — %d %s, %s", count, call, formatDuration(elapsedSeconds))
}

var sparkBlocks = []rune("▁▂▃▄▅▆▇█")

// sparkline renders shape, not value, per lucida's own rule -- normalized
// to the shown window's own min/max, not any fixed scale. A flat series
// (max == min) renders as a level mid-height line rather than dividing by
// zero.
func sparkline(values []float64) string {
	lo, hi := values[0], values[0]
	for _, v := range values {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	var b strings.Builder
	for _, v := range values {
		idx := len(sparkBlocks) / 2
		if hi > lo {
			idx = int((v - lo) / (hi - lo) * float64(len(sparkBlocks)-1))
		}
		b.WriteRune(sparkBlocks[idx])
	}
	return b.String()
}

// maxLineChars backstops truncateWords' word-boundary cut. strings.Fields
// treats any whitespace-free run -- a URL, a long CJK sentence (CJK text
// doesn't use inter-word spaces the way Latin scripts do) -- as a single
// "word", so the word-count cap never fires on it. A field this long
// defeats the bar/tradeoffCol alignment math in render() regardless of
// whether it "counts" as one word or twenty, so this is a hard character
// cap under the word cap, not a replacement for it.
const maxLineChars = 140

// truncateWords cuts s to at most n words, appending "…" if anything was
// dropped, then backstops with truncateWordsCharCap(s, maxLineChars) --
// the ordinary case for every caller that doesn't need a smaller char
// budget than the file-wide default.
func truncateWords(s string, n int) string {
	return truncateWordsCharCap(s, n, maxLineChars)
}

// truncateWordsCharCap is truncateWords with an explicit character cap
// instead of the file-wide maxLineChars default -- added (2026-08-22) for
// the one caller (a Consequence's "next" text with a tool tally appended
// after it) that needs a SMALLER char budget than every other line gets,
// to leave room for that suffix. Word-count truncation alone doesn't
// catch this case: a technical field can sit entirely under the word cap
// while still running long in characters (long identifiers, hyphenated
// names), which is exactly what defeated a word-based reservation here
// (found live, 2026-08-22: a real 16-word Next field with long
// identifiers rendered at the full word budget -- 16 is not > 16 -- so a
// word-count reservation never fired at all, and the tally suffix still
// pushed that line to 162 visible chars against every other line's own
// 140-char cap).
func truncateWordsCharCap(s string, n, charCap int) string {
	words := strings.Fields(s)
	if len(words) > n {
		s = strings.Join(words[:n], " ") + "…"
	}
	if utf8.RuneCountInString(s) > charCap {
		runes := []rune(s)
		if charCap < 1 {
			charCap = 1
		}
		s = string(runes[:charCap-1]) + "…"
	}
	// escapeMarkdown runs last, after truncation math, not before -- the
	// char cap above operates on real content length; escaping first would
	// let truncation cut a line right between a backslash it just added
	// and the character that backslash was escaping, and would count the
	// cap against inflated (escaped) length instead of the content a
	// reader actually judges the line's length by.
	return escapeMarkdown(s)
}

// markdownEscaper escapes the three characters most likely to visibly
// corrupt the transcript when Claude-authored free text is embedded in a
// markdown report -- an unmatched backtick or a run of asterisks/
// underscores can open a code span or emphasis run that swallows
// everything after it on the page. Applied at render time only, never
// baked into status-input.json itself (sanitizeStatusData's control-char
// stripping IS baked in, because a control character is never legitimate
// content; a backtick in someone's real text is, so escaping it is a
// rendering concern, not a data-cleaning one -- baking it in would also
// double-escape on the next prune-and-rewrite cycle).
var markdownEscaper = strings.NewReplacer("`", "\\`", "*", "\\*", "_", "\\_")

func escapeMarkdown(s string) string {
	return markdownEscaper.Replace(s)
}

// joinTruncated renders up to maxItems entries from items as one line,
// joined with "; " -- keeping at most the first maxItems, silent keep-
// first, same shape as every other cap in this file, not a "N more"
// marker. Each kept item is truncated to perItemWords INDIVIDUALLY,
// before joining, not after: joining first and truncating the whole
// joined blob to one shared word budget was silently destroying real
// content (found live, 2026-08-22, against this project's own
// status-input.json -- a longer first factor consumed the entire budget
// and a genuinely load-bearing second factor was cut to three words and
// vanished, not just shortened). Per-item truncation means a kept item is
// always either complete or cleanly "…"-cut on its own, never annihilated
// by a sibling's overflow. Semicolon, not comma: several real items
// already contain internal commas of their own ("schema, render, tests"),
// so a comma-only join would make it ambiguous where one item ends and
// the next begins.
func joinTruncated(items []string, maxItems, perItemWords int) string {
	if len(items) > maxItems {
		items = items[:maxItems]
	}
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = truncateWords(it, perItemWords)
	}
	return strings.Join(parts, "; ")
}

// trendSeries bundles this render's two momentum series -- ratio (the
// provisional fraction) and calls (tool-call count per goal) -- into one
// value rather than two trailing []float64 params. Both are the same
// underlying type, so two separate params would let a caller transpose
// them without the compiler ever catching it; a named field makes the
// mistake a type-checked one instead.
type trendSeries struct {
	ratio []float64
	calls []float64
}

// cappedChains/cappedContributions return d's own Chains/Contributions
// already capped to maxChains/maxContributions -- shared between render()
// and validateStatusData so validation runs against exactly the entries
// that get shown, not entries a cap would have already dropped before a
// reader ever saw them.
func cappedChains(d statusdata.StatusData) []statusdata.Chain {
	c := d.Chains
	if len(c) > maxChains {
		c = c[len(c)-maxChains:]
	}
	return c
}

func cappedContributions(d statusdata.StatusData) []statusdata.Contribution {
	c := d.Contributions
	if len(c) > maxContributions {
		c = c[len(c)-maxContributions:]
	}
	return c
}

// validateStatusData checks the two things render() depends on but can't
// recover from -- a chain with too few steps, or a contribution missing
// pushing/constraining/a valid durability -- against the already-capped
// views (cappedChains/cappedContributions), same as render() checked
// inline before this was extracted. Checked in this order (chains before
// contributions) so a multi-invalid-field input surfaces the same first
// error render() would have found rendering top to bottom.
func validateStatusData(d statusdata.StatusData) error {
	for _, c := range cappedChains(d) {
		if len(c.Steps) < statusdata.MinChainSteps {
			return fmt.Errorf("chain %q: too few steps (%d) -- a 1-2 step sequence reads as prose, use context or consequences instead", c.Label, len(c.Steps))
		}
	}
	for _, c := range cappedContributions(d) {
		if len(c.Pushing) == 0 {
			return fmt.Errorf("contribution %q: pushing must name at least one factor", c.Decision)
		}
		if len(c.Constraining) == 0 {
			return fmt.Errorf("contribution %q: constraining must name at least one factor", c.Decision)
		}
		if c.Durability != "provisional" && c.Durability != "durable" {
			return fmt.Errorf("contribution %q: durability must be \"provisional\" or \"durable\", got %q", c.Decision, c.Durability)
		}
	}
	return nil
}

// render renders status-input.json as markdown -- status.txt's own format,
// which status-fallback.sh's Stop hook drops into Claude Code's own
// transcript via the hook's systemMessage field, where it renders as real
// markdown. A header line (timestamp + over-budget diagnostic) leads,
// followed by one section order (Drivers/Constraints -> Context -> Chains
// -> Contributions [+ provisional-trend sparkline] -> Consequences [+
// tool-call-velocity sparkline]), one validateStatusData check. toolTally
// is pre-formatted (formatToolTally)
// and, when non-empty, appended to only the LAST Consequence shown -- the
// currently-active goal's own line, not every Consequence still visible
// under the cap and not leads_to. Empty string when the caller has nothing
// to show (no -out, no Consequences, or the tool-count sidecar couldn't be
// read) -- render() itself never reads that file; main() decides whether
// this render gets a tally at all and hands it in pre-formatted, keeping
// render() a pure function of its arguments like every other section here.
func render(d statusdata.StatusData, trend trendSeries, toolTally string) (string, error) {
	if err := validateStatusData(d); err != nil {
		return "", err
	}
	var b strings.Builder

	drivers := d.Drivers
	if len(drivers) > maxStandingFactors {
		drivers = drivers[len(drivers)-maxStandingFactors:]
	}
	constraints := d.Constraints
	if len(constraints) > maxStandingFactors {
		constraints = constraints[len(constraints)-maxStandingFactors:]
	}
	if len(drivers) > 0 {
		fmt.Fprintf(&b, "- ↑ %s\n", joinTruncated(drivers, maxStandingFactors, maxStandingFactorWords))
	}
	if len(constraints) > 0 {
		fmt.Fprintf(&b, "- ↓ %s\n", joinTruncated(constraints, maxStandingFactors, maxStandingFactorWords))
	}

	context := d.Context
	if len(context) > maxContextLines {
		context = context[len(context)-maxContextLines:]
	}
	for _, line := range context {
		fmt.Fprintf(&b, "- ☑ %s\n", truncateWords(line, maxLineWords))
	}

	for _, c := range cappedChains(d) {
		if c.Label != "" {
			fmt.Fprintf(&b, "- ⛓ %s\n", truncateWords(c.Label, maxLineWords))
		}
		for i, step := range c.Steps {
			step = truncateWords(step, maxLineWords)
			connector := "├─"
			switch i {
			case 0:
				connector = "┌─"
			case len(c.Steps) - 1:
				connector = "└─"
			}
			// Leading "- " is load-bearing, not decoration: without a list
			// marker, CommonMark treats an indented non-blank line right
			// after a list item as a lazy-continuation of THAT item's own
			// paragraph, not a new one -- a compliant renderer would fold
			// every step into the chain label's own bullet (or the
			// previous section's, if there's no label) instead of showing
			// one flowchart node per line.
			fmt.Fprintf(&b, "- %s %s\n", connector, step)
		}
	}

	contributions := cappedContributions(d)
	if len(contributions) > 0 {
		b.WriteString("\n**Contributions**\n")
		for _, c := range contributions {
			fmt.Fprintf(&b, "- %s %s\n", statusGlyph(c.Durability), truncateWords(c.Decision, maxLineWords))
			fmt.Fprintf(&b, "  - ↑ %s\n", joinTruncated(c.Pushing, maxFactorsPerList, maxLineWords))
			fmt.Fprintf(&b, "  - ↓ %s\n", joinTruncated(c.Constraining, maxFactorsPerList, maxLineWords))
		}
		if len(trend.ratio) >= minSparkPoints {
			fmt.Fprintf(&b, "\n`provisional trend %s %.0f%%`\n", sparkline(trend.ratio), trend.ratio[len(trend.ratio)-1]*100)
		}
	}

	if len(d.Consequences) > 0 {
		consequences := d.Consequences
		if len(consequences) > maxConsequences {
			consequences = consequences[len(consequences)-maxConsequences:]
		}
		b.WriteString("\n**Consequences**\n")
		for i, c := range consequences {
			tally := ""
			if i == len(consequences)-1 {
				tally = toolTally
			}
			nextCharCap := maxLineChars
			if tally != "" {
				nextCharCap = maxLineChars - tallyReserveChars
			}
			if c.Risky {
				fmt.Fprintf(&b, "- ⚠ %s%s\n", truncateWordsCharCap(c.Next, maxLineWords, nextCharCap), tally)
			} else {
				fmt.Fprintf(&b, "- ☐ %s%s\n", truncateWordsCharCap(c.Next, maxLineWords, nextCharCap), tally)
			}
			fmt.Fprintf(&b, "  ↳ %s\n", truncateWords(c.LeadsTo, maxLineWords))
		}
		if len(trend.calls) >= minSparkPoints {
			latest := trend.calls[len(trend.calls)-1]
			word := "calls"
			if latest == 1 {
				word = "call"
			}
			fmt.Fprintf(&b, "\n`tool-call velocity %s %.0f %s`\n", sparkline(trend.calls), latest, word)
		}
	}

	var over int
	for _, f := range statusdata.ExtractFields(d) {
		if _, isOver := statusdata.OverBudget(f); isOver {
			over++
		}
	}
	diag := ""
	if over > 0 {
		word := "fields"
		if over == 1 {
			word = "field"
		}
		diag = fmt.Sprintf(" — %d %s over budget", over, word)
	}

	header := fmt.Sprintf("### pellicle — updated %s%s\n\n", time.Now().Format("15:04:05"), diag)
	return header + b.String(), nil
}

// initTemplate is what -init scaffolds for a project adopting pellicle for
// the first time -- a minimal, clearly-placeholder example of each section's
// shape, not real content. Real content is Claude's job to fill in per turn.
// risky is optional (json:"...,omitempty") -- shown here so the shape is
// discoverable, but most Consequences aren't risky, so omitting it is the
// common case, not a gap to fill in. risky_command is a second optional
// Consequence field, left out of the scaffold below on the same basis --
// set only when a risky next action reduces to one specific, identifiable
// shell command (a literal substring like "push --force", not prose); it's
// what status-risk-gate.sh matches against to escalate that one command to
// a permission prompt, not something every risky Consequence needs.
const initTemplate = `{
  "context": [
    "REPLACE ME: one settled fact per line, newest last"
  ],
  "contributions": [
    {
      "decision": "REPLACE ME: a specific decision that actually happened",
      "pushing": ["REPLACE ME: a specific factor driving toward this"],
      "constraining": ["REPLACE ME: a specific factor bounding or limiting it"],
      "durability": "provisional"
    }
  ],
  "consequences": [
    {
      "next": "REPLACE ME: the actual next action",
      "leads_to": "REPLACE ME: where that action leads",
      "risky": false
    }
  ]
}
`

func main() {
	out := flag.String("out", "", "output path (default: stdout)")
	showVersion := flag.Bool("version", false, "print version and exit")
	initFlag := flag.Bool("init", false, "write a starter status-input.json to the current directory and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("render-status", buildVersion())
		return
	}

	if *initFlag {
		const path = "status-input.json"
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(os.Stderr, "render-status: %s already exists, not overwriting\n", path)
			os.Exit(1)
		}
		if err := os.WriteFile(path, []byte(initTemplate), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "render-status: write %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s -- fill in real content, then render with: render-status -out status.txt %s\n", path, path)
		return
	}

	var raw []byte
	var err error
	var inputPath string
	if args := flag.Args(); len(args) > 0 {
		inputPath = args[0]
		raw, err = os.ReadFile(inputPath)
	} else {
		raw, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "render-status: read input: %v\n", err)
		os.Exit(1)
	}

	var d statusdata.StatusData
	if err := json.Unmarshal(raw, &d); err != nil {
		fmt.Fprintf(os.Stderr, "render-status: parse JSON: %v\n", err)
		os.Exit(1)
	}
	d = sanitizeStatusData(d)
	d = capStatusData(d)

	// Tool-call tally: computed whenever there's an actual current goal to
	// attach it to -- no Consequences means nothing for the tally to be
	// "next to". .pellicle-tool-count.json is read from the current
	// directory, not derived from *out (see toolCountFile's own comment) --
	// status-tool-count.sh writes the same literal relative path.
	var toolTally string
	var tc toolCount
	var haveTally bool
	if len(d.Consequences) > 0 {
		now := time.Now().Unix()
		goal := d.Consequences[len(d.Consequences)-1].Next
		var err error
		tc, err = loadToolTally(toolCountFile, goal, now)
		if err != nil {
			fmt.Fprintf(os.Stderr, "render-status: write %s: %v\n", toolCountFile, err)
		}
		d = expireStaleConsequence(d, tc)
		if len(d.Consequences) > 0 {
			toolTally = formatToolTally(tc.Count, now-tc.Since)
			haveTally = true
		}
	}

	// History only applies when writing a real status.txt -- stdout has no
	// stable sibling path to derive status-history.jsonl from, and a
	// stdout-only call is typically a preview/dry-run, not a real tick worth
	// recording. ratio and calls are each independently nil when this render
	// has nothing to say about that metric (no Contributions for ratio, no
	// live tally for calls) -- appendHistory skips the point only if BOTH
	// are nil, so status-fallback.sh's stub (neither) still appends
	// nothing, but a render with only one of the two still gets that one
	// metric recorded, not silently dropped for lacking the other.
	var trend trendSeries
	if *out != "" {
		var ratio *float64
		if r, ok := provisionalRatio(d.Contributions); ok {
			ratio = &r
		}
		var calls *int
		if haveTally {
			calls = &tc.Count
		}
		hpath := historyPath(*out)
		if err := appendHistory(hpath, ratio, calls); err != nil {
			fmt.Fprintf(os.Stderr, "render-status: append history: %v\n", err)
		}
		if pts, err := readHistory(hpath); err == nil {
			trend = trendSeries{ratio: ratioSeries(pts), calls: callsSeries(pts)}
		}
	}

	rendered, err := render(d, trend, toolTally)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render-status: %v\n", err)
		os.Exit(1)
	}

	if *out == "" {
		fmt.Print(rendered)
		return
	}
	if err := atomicWriteFile(*out, []byte(rendered), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "render-status: write %s: %v\n", *out, err)
		os.Exit(1)
	}

	// Prune the input file itself down to what capStatusData already kept,
	// not just the rendered view -- without this, render()'s own
	// keep-newest caps only ever trimmed what status.txt showed, while
	// status-input.json kept every entry ever written, forever. That's
	// why lint-content's backlog only ever grew: it was flagging fields
	// no render had shown in dozens of turns, not fields anyone would
	// actually see truncated. Only when given a real file path -- stdin
	// input (status-fallback.sh's synthetic git-fallback JSON) has no
	// sibling path to write back to, and isn't the real status-input.json
	// content to begin with.
	if inputPath != "" {
		pruned, err := marshalPrunedInput(d)
		if err != nil {
			fmt.Fprintf(os.Stderr, "render-status: marshal pruned %s: %v\n", inputPath, err)
			os.Exit(1)
		}
		if err := atomicWriteFile(inputPath, pruned, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "render-status: write pruned %s: %v\n", inputPath, err)
			os.Exit(1)
		}
	}
}
