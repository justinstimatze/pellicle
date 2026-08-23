// lint-content checks status-input.json's free-text fields for length
// before render-status ever gets a chance to silently truncate one.
//
// render-status truncates a too-long field with a trailing "…" -- a
// reasonable safety valve for a genuine outlier, but it doesn't tell the
// author (Claude, writing status-input.json) that a field ran over budget
// until someone reads the rendered report and notices the ellipsis. This
// checks the same budget at write time instead: every field render()
// would truncate gets word-counted with the exact same strings.Fields
// logic render-status's own truncateWords uses.
//
// This used to shell out to cope (github.com/justinstimatze/cope)'s
// cope-gate --check, repurposing its card mechanism as a general text
// checker. That measured real: cope's own scan.go confirmed the premise
// worked mechanically. But cope's word counter tokenizes via a fixed
// [A-Za-z']+ regex that splits on every hyphen and period ("cope-gate"
// counted as 2 words, "status-risk-gate.sh" as 4) -- not configurable
// per-card -- while render()'s own truncateWords counts via plain
// strings.Fields and doesn't split those at all. A "mirrors
// render-status's own maxLineWords" check that counts differently than
// the thing it mirrors isn't actually checking the real budget, and
// every one of the 10 built-in reply-shape rules cope also offers was
// permanently gated off anyway (a status field is a single declarative
// fact, never a multi-paragraph reply) -- cope's card mechanism had
// nothing left to contribute once the one active rule was wrong. Doing
// the count natively, in Go, against the exact function render() uses,
// fixes the mismatch directly and drops a second binary dependency that
// was a silent no-op whenever it wasn't on PATH. Full story in
// CHANGELOG.md.
//
// The field/extractFields/overBudget mechanism itself now lives in
// internal/statusdata (Field/ExtractFields/OverBudget) -- render-status's
// own diagnostic strip counts over-budget fields the same way, and a
// third copy of this logic drifting from either one is exactly the bug
// this file's own history is about.
//
// Wired into status-content-gate.sh, a PostToolUse hook (see the repo
// root) that runs this after every Edit/Write of status-input.json and
// surfaces any over-budget field back to Claude before render-status ever
// gets a chance to truncate it. Also runnable by hand for the same check.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/justinstimatze/pellicle/internal/statusdata"
)

// version is "dev" by default and baked at release time via
//
//	go install -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/lint-content
//
// Same pattern as render-status's own version var -- the git tag is the
// single source of truth, resolved by buildVersion().
var version = "dev"

// buildVersion reports the binary's version, preferring (in order): a
// release value baked in via -ldflags; the module version when installed
// with `go install …@vX.Y.Z`; the embedded VCS commit (+dirty) for local
// `go build`. Falls back to "dev" when none is available.
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

func main() {
	path := flag.String("path", "status-input.json", "status-input.json to check")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("lint-content", buildVersion())
		return
	}

	raw, err := os.ReadFile(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lint-content: read %s: %v\n", *path, err)
		os.Exit(1)
	}
	var d statusdata.StatusData
	if err := json.Unmarshal(raw, &d); err != nil {
		fmt.Fprintf(os.Stderr, "lint-content: parse %s: %v\n", *path, err)
		os.Exit(1)
	}

	var over int
	for _, f := range statusdata.ExtractFields(d) {
		if f.Text == "" {
			continue
		}
		words, isOver := statusdata.OverBudget(f)
		if !isOver {
			continue
		}
		over++
		fmt.Printf("%s: %d words, budget %d: %q\n", f.Label, words, f.Budget, f.Text)
	}

	if over > 0 {
		fmt.Fprintf(os.Stderr, "lint-content: %d field(s) over render-status's own length budget\n", over)
		os.Exit(1)
	}
	fmt.Println("lint-content: clean")
}
