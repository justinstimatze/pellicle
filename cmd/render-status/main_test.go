package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/justinstimatze/pellicle/internal/statusdata"
)

func TestProvisionalRatio(t *testing.T) {
	if _, ok := provisionalRatio(nil); ok {
		t.Error("empty Contributions: want ok=false")
	}
	cs := []statusdata.Contribution{
		{Durability: "provisional"},
		{Durability: "provisional"},
		{Durability: "durable"},
		{Durability: "durable"},
	}
	ratio, ok := provisionalRatio(cs)
	if !ok {
		t.Fatal("want ok=true")
	}
	if want := 0.5; ratio < want-1e-9 || ratio > want+1e-9 {
		t.Errorf("ratio=%v, want %v (2 of 4 provisional)", ratio, want)
	}
}

func TestSparklineFlatSeries(t *testing.T) {
	out := sparkline([]float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5})
	// A flat series must not divide by zero and must use the mid-height
	// block for every point.
	mid := sparkBlocks[len(sparkBlocks)/2]
	count := strings.Count(out, string(mid))
	if count != 6 {
		t.Errorf("flat series: got %d mid-height blocks, want 6 (output: %q)", count, out)
	}
}

func TestSparklineRange(t *testing.T) {
	out := sparkline([]float64{0.0, 0.5, 1.0})
	// Lowest and highest points should map to the first and last block.
	runes := []rune(out)
	if runes[0] != sparkBlocks[0] {
		t.Errorf("min value: got block %q, want %q", runes[0], sparkBlocks[0])
	}
	if runes[len(runes)-1] != sparkBlocks[len(sparkBlocks)-1] {
		t.Errorf("max value: got block %q, want %q", runes[len(runes)-1], sparkBlocks[len(sparkBlocks)-1])
	}
}

func TestHistoryPath(t *testing.T) {
	got := historyPath("/a/b/status.txt")
	want := "/a/b/status-history.jsonl"
	if got != want {
		t.Errorf("historyPath = %q, want %q", got, want)
	}
}

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }

func TestAppendReadHistoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status-history.jsonl")

	if pts, err := readHistory(path); err != nil || pts != nil {
		t.Fatalf("readHistory on missing file: pts=%v err=%v, want nil, nil", pts, err)
	}

	for _, r := range []float64{0.1, 0.5, 0.9} {
		if err := appendHistory(path, fp(r), nil); err != nil {
			t.Fatalf("appendHistory: %v", err)
		}
	}
	pts, err := readHistory(path)
	if err != nil {
		t.Fatalf("readHistory: %v", err)
	}
	got := ratioSeries(pts)
	want := []float64{0.1, 0.5, 0.9}
	if len(got) != len(want) {
		t.Fatalf("got %d points, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] < w-1e-9 || got[i] > w+1e-9 {
			t.Errorf("point %d = %v, want %v", i, got[i], w)
		}
	}
}

func TestAppendHistorySkipsAPointWithNeitherMetric(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status-history.jsonl")
	if err := appendHistory(path, nil, nil); err != nil {
		t.Fatalf("appendHistory: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a point with neither ratio nor calls should not create the file at all")
	}
}

func TestAppendHistoryRecordsRatioAndCallsIndependently(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status-history.jsonl")
	// A real render can have Contributions but no live tally, calls but no
	// Contributions, or both -- three points here cover all three, and
	// each series should only pick up the points that actually set it.
	if err := appendHistory(path, fp(0.2), nil); err != nil {
		t.Fatal(err)
	}
	if err := appendHistory(path, nil, ip(5)); err != nil {
		t.Fatal(err)
	}
	if err := appendHistory(path, fp(0.4), ip(9)); err != nil {
		t.Fatal(err)
	}
	pts, err := readHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ratioSeries(pts), []float64{0.2, 0.4}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("ratioSeries = %v, want %v", got, want)
	}
	if got, want := callsSeries(pts), []float64{5, 9}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("callsSeries = %v, want %v", got, want)
	}
}

func TestReadHistoryCapsAtMaxSparkPoints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status-history.jsonl")
	for i := 0; i < maxSparkPoints+10; i++ {
		if err := appendHistory(path, fp(float64(i)), nil); err != nil {
			t.Fatalf("appendHistory: %v", err)
		}
	}
	pts, err := readHistory(path)
	if err != nil {
		t.Fatalf("readHistory: %v", err)
	}
	got := ratioSeries(pts)
	if len(got) != maxSparkPoints {
		t.Fatalf("got %d points, want %d (cap)", len(got), maxSparkPoints)
	}
	// Oldest-first, newest kept: the last point should be the last appended.
	if want := float64(maxSparkPoints + 9); got[len(got)-1] != want {
		t.Errorf("newest point = %v, want %v (oldest should be dropped, not newest)", got[len(got)-1], want)
	}
}

func TestReadHistorySkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status-history.jsonl")
	content := `{"time":1,"ratio":0.1}
not json
{"time":2,"ratio":0.2}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	pts, err := readHistory(path)
	if err != nil {
		t.Fatalf("readHistory: %v", err)
	}
	if len(pts) != 2 {
		t.Fatalf("got %d points, want 2 (malformed line skipped)", len(pts))
	}
}

func TestTruncateWords(t *testing.T) {
	short := "three word phrase"
	if got := truncateWords(short, 10); got != short {
		t.Errorf("truncateWords(%q, 10) = %q, want unchanged", short, got)
	}

	long := "one two three four five six seven eight nine ten eleven twelve"
	got := truncateWords(long, 10)
	want := "one two three four five six seven eight nine ten…"
	if got != want {
		t.Errorf("truncateWords(long, 10) = %q, want %q", got, want)
	}
}

func TestRenderTruncatesLongContextLine(t *testing.T) {
	var words []string
	for i := 1; i <= maxLineWords+5; i++ {
		words = append(words, fmt.Sprintf("word%d", i))
	}
	long := strings.Join(words, " ")
	out, err := render(statusdata.StatusData{Context: []string{long}}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	over := fmt.Sprintf("word%d", maxLineWords+1)
	if strings.Contains(out, over) {
		t.Errorf("%s present -- render should have truncated at maxLineWords (%d)", over, maxLineWords)
	}
	if !strings.Contains(out, "…") {
		t.Error("no truncation marker -- a cut line should end in an ellipsis")
	}
}

func TestRenderDiagnosticSilentWhenNothingOverBudget(t *testing.T) {
	out, err := render(statusdata.StatusData{Context: []string{"a short settled fact"}}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "over budget") {
		t.Errorf("clean input should not mention \"over budget\": %s", out)
	}
}

func TestRenderDiagnosticCountsFieldsAboutToBeTruncated(t *testing.T) {
	var words []string
	for i := 1; i <= maxLineWords+5; i++ {
		words = append(words, fmt.Sprintf("word%d", i))
	}
	long := strings.Join(words, " ")
	d := statusdata.StatusData{Context: []string{long}, Consequences: []statusdata.Consequence{{Next: long, LeadsTo: "x"}}}
	out, err := render(d, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2 fields over budget") {
		t.Errorf("expected \"2 fields over budget\" in divider, got: %s", out)
	}
}

func TestRenderShowsDriversAndConstraints(t *testing.T) {
	d := statusdata.StatusData{
		Drivers:     []string{"ship by EOD"},
		Constraints: []string{"can't use this library, its license blocks us"},
		Context:     []string{"x"},
	}
	out, err := render(d, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "- ↑ ") || !strings.Contains(out, "ship by EOD") {
		t.Error("drivers line missing from output")
	}
	if !strings.Contains(out, "- ↓ ") || !strings.Contains(out, "can't use this library") {
		t.Error("constraints line missing from output")
	}
}

func TestRenderOmitsDriversAndConstraintsWhenAbsent(t *testing.T) {
	out, err := render(statusdata.StatusData{Context: []string{"x"}}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "- ↑ ") || strings.Contains(out, "- ↓ ") {
		t.Error("no Drivers/Constraints in input: lines should not appear")
	}
}

func TestRenderCapsStandingFactors(t *testing.T) {
	var drivers []string
	for i := 0; i < maxStandingFactors+3; i++ {
		drivers = append(drivers, fmt.Sprintf("driver%d", i))
	}
	out, err := render(statusdata.StatusData{Drivers: drivers}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	over := "driver0"
	if strings.Contains(out, over) {
		t.Errorf("%s present -- cap should have kept only the newest %d drivers", over, maxStandingFactors)
	}
	last := fmt.Sprintf("driver%d", maxStandingFactors+2)
	if !strings.Contains(out, last) {
		t.Error("newest driver missing -- cap should keep the newest, not the oldest")
	}
}

func TestRenderDriversImmediatelyPrecedeContext(t *testing.T) {
	// Markdown renders Drivers/Constraints/Context as one continuous bullet
	// list -- a blank line mid-list would be non-idiomatic markdown, unlike
	// the old ANSI report's own visual breathing-room convention. The thing
	// that still matters is order: drivers before context, adjacent.
	out, err := render(statusdata.StatusData{Drivers: []string{"d"}, Context: []string{"c"}}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out, "\n")
	var driversIdx, contextIdx = -1, -1
	for i, l := range lines {
		if strings.HasPrefix(l, "- ↑ ") {
			driversIdx = i
		}
		if strings.Contains(l, "☑") {
			contextIdx = i
		}
	}
	if driversIdx < 0 || contextIdx < 0 {
		t.Fatal("expected both a drivers line and a context line")
	}
	if contextIdx != driversIdx+1 {
		t.Errorf("expected drivers and context on adjacent lines, got %d lines apart", contextIdx-driversIdx)
	}
}

func TestRenderContextCap(t *testing.T) {
	var lines []string
	for i := 0; i < 15; i++ {
		lines = append(lines, "line "+string(rune('a'+i)))
	}
	out, err := render(statusdata.StatusData{Context: lines}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, "☑"); n != maxContextLines {
		t.Errorf("got %d context lines rendered, want %d (cap)", n, maxContextLines)
	}
	if !strings.Contains(out, "line "+string(rune('a'+14))) {
		t.Error("newest line missing -- cap should keep the newest, not the oldest")
	}
	if strings.Contains(out, "line a\n") {
		t.Error("oldest line present -- cap should have dropped it")
	}
}

func TestRenderContributionsCap(t *testing.T) {
	var cs []statusdata.Contribution
	for i := 0; i < maxContributions+3; i++ {
		cs = append(cs, statusdata.Contribution{
			Decision: "decision " + string(rune('a'+i)),
			Pushing:  []string{"p"}, Constraining: []string{"c"}, Durability: "durable",
		})
	}
	out, err := render(statusdata.StatusData{Contributions: cs}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, "●"); n != maxContributions {
		t.Errorf("got %d contributions rendered, want %d (cap)", n, maxContributions)
	}
	last := "decision " + string(rune('a'+maxContributions+2))
	if !strings.Contains(out, last) {
		t.Error("newest contribution missing -- cap should keep the newest, not the oldest")
	}
	if strings.Contains(out, "decision a\n") {
		t.Error("oldest contribution present -- cap should have dropped it")
	}
}

func TestRenderShowsPushingAndConstraining(t *testing.T) {
	cs := []statusdata.Contribution{
		{Decision: "d1", Pushing: []string{"ships faster"}, Constraining: []string{"only two hours before the demo"}, Durability: "durable"},
	}
	out, err := render(statusdata.StatusData{Contributions: cs}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "  - ↑ ") || !strings.Contains(out, "ships faster") {
		t.Error("pushing line missing from output")
	}
	if !strings.Contains(out, "  - ↓ ") || !strings.Contains(out, "only two hours before the demo") {
		t.Error("constraining line missing from output")
	}
}

func TestRenderKeepsOnlyFirstFactorPerDirection(t *testing.T) {
	// maxFactorsPerList == 1: real pushing/constraining items run long
	// enough (17-word median, measured against this project's own
	// status-input.json) that joining a second or third onto one line and
	// truncating the joined blob was silently destroying content, not
	// shortening it -- see joinTruncated's own comment. A second factor is
	// now dropped whole, not half-rendered.
	cs := []statusdata.Contribution{
		{Decision: "d1", Pushing: []string{"factor A", "factor B"}, Constraining: []string{"limit X", "limit Y"}, Durability: "durable"},
	}
	out, err := render(statusdata.StatusData{Contributions: cs}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "factor A") {
		t.Error("first pushing factor missing")
	}
	if strings.Contains(out, "factor B") {
		t.Error("second pushing factor rendered -- maxFactorsPerList should keep only the first")
	}
	if !strings.Contains(out, "limit X") {
		t.Error("first constraining factor missing")
	}
	if strings.Contains(out, "limit Y") {
		t.Error("second constraining factor rendered -- maxFactorsPerList should keep only the first")
	}
}

func TestRenderCapsFactorsPerList(t *testing.T) {
	var pushing []string
	for i := 0; i < maxFactorsPerList+3; i++ {
		pushing = append(pushing, fmt.Sprintf("factor%d", i))
	}
	cs := []statusdata.Contribution{{Decision: "d", Pushing: pushing, Constraining: []string{"c"}, Durability: "durable"}}
	out, err := render(statusdata.StatusData{Contributions: cs}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	over := fmt.Sprintf("factor%d", maxFactorsPerList)
	if strings.Contains(out, over) {
		t.Errorf("%s present -- pushing should have kept only the first %d factors", over, maxFactorsPerList)
	}
	if !strings.Contains(out, "factor0") {
		t.Error("first factor missing -- cap should keep the first factors, not drop them all")
	}
}

func TestRenderRiskyConsequenceUsesWarningGlyph(t *testing.T) {
	cs := []statusdata.Consequence{
		{Next: "routine cleanup", LeadsTo: "l"},
		{Next: "force-push to main", LeadsTo: "l", Risky: true},
	}
	out, err := render(statusdata.StatusData{Consequences: cs}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "⚠ force-push to main") {
		t.Error("risky Consequence should render with the ⚠ glyph")
	}
	if !strings.Contains(out, "☐ routine cleanup") {
		t.Error("non-risky Consequence should still render with the plain ☐ glyph")
	}
}

func TestRenderAcceptsRiskyCommandWithoutDisplayingIt(t *testing.T) {
	// risky_command is operational metadata for status-risk-gate.sh, not
	// reader-facing content -- render() should accept the field (not choke
	// on it) but never print the literal command text into status.txt.
	cs := []statusdata.Consequence{
		{Next: "force-push to main", LeadsTo: "l", Risky: true, RiskyCommand: "push --force"},
	}
	out, err := render(statusdata.StatusData{Consequences: cs}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "⚠ force-push to main") {
		t.Error("risky Consequence with risky_command set should still render with the ⚠ glyph")
	}
	if strings.Contains(out, "push --force") {
		t.Error("risky_command text should not appear in rendered output -- it's operational metadata, not reader-facing content")
	}
}

func TestRenderConsequencesCap(t *testing.T) {
	var cs []statusdata.Consequence
	for i := 0; i < maxConsequences+3; i++ {
		cs = append(cs, statusdata.Consequence{Next: "next " + string(rune('a'+i)), LeadsTo: "leads to"})
	}
	out, err := render(statusdata.StatusData{Consequences: cs}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, "- ☐ "); n != maxConsequences {
		t.Errorf("got %d consequences rendered, want %d (cap)", n, maxConsequences)
	}
	last := "next " + string(rune('a'+maxConsequences+2))
	if !strings.Contains(out, last) {
		t.Error("newest consequence missing -- cap should keep the newest, not the oldest")
	}
	if strings.Contains(out, "next a\n") {
		t.Error("oldest consequence present -- cap should have dropped it")
	}
}

func TestRenderContributionValidation(t *testing.T) {
	_, err := render(statusdata.StatusData{Contributions: []statusdata.Contribution{
		{Decision: "d", Constraining: []string{"c"}, Durability: "durable"},
	}}, trendSeries{}, "")
	if err == nil {
		t.Error("empty pushing: want an error, got nil")
	}

	_, err = render(statusdata.StatusData{Contributions: []statusdata.Contribution{
		{Decision: "d", Pushing: []string{"p"}, Durability: "durable"},
	}}, trendSeries{}, "")
	if err == nil {
		t.Error("empty constraining: want an error, got nil")
	}

	_, err = render(statusdata.StatusData{Contributions: []statusdata.Contribution{
		{Decision: "d", Pushing: []string{"p"}, Constraining: []string{"c"}, Durability: "sort-of"},
	}}, trendSeries{}, "")
	if err == nil {
		t.Error("invalid durability: want an error, got nil")
	}
}

func TestRenderChainTooFewSteps(t *testing.T) {
	_, err := render(statusdata.StatusData{Chains: []statusdata.Chain{
		{Label: "l", Steps: []string{"a", "b"}},
	}}, trendSeries{}, "")
	if err == nil {
		t.Error("2-step chain: want an error (below minChainSteps), got nil")
	}
}

func TestRenderChainRendering(t *testing.T) {
	out, err := render(statusdata.StatusData{Chains: []statusdata.Chain{
		{Label: "cause", Steps: []string{"first", "second", "third"}},
	}}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cause") {
		t.Error("label missing from output")
	}
	if !strings.Contains(out, "┌─") || !strings.Contains(out, "first") {
		t.Error("first step should use the ┌─ connector")
	}
	if !strings.Contains(out, "├─") || !strings.Contains(out, "second") {
		t.Error("a middle step should use the ├─ connector")
	}
	if !strings.Contains(out, "└─") || !strings.Contains(out, "third") {
		t.Error("last step should use the └─ connector")
	}
	if i, j := strings.Index(out, "┌─"), strings.Index(out, "first"); i < 0 || j < i {
		t.Error("┌─ should precede \"first\" in the output")
	}
	// Each step needs its own markdown list marker -- without one, a
	// CommonMark-compliant renderer treats a non-blank line right after a
	// list item as a lazy continuation of that item's own paragraph, not a
	// new list item, and folds every step into the chain label's bullet.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "┌─") || strings.Contains(line, "├─") || strings.Contains(line, "└─") {
			if !strings.HasPrefix(strings.TrimSpace(line), "-") {
				t.Errorf("chain step line %q has no leading markdown list marker", line)
			}
		}
	}
}

func TestRenderChainOmittedWhenAbsent(t *testing.T) {
	out, err := render(statusdata.StatusData{Context: []string{"x"}}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "┌─") || strings.Contains(out, "├─") || strings.Contains(out, "└─") {
		t.Error("no Chains in input: chain connector glyphs should not appear")
	}
}

func TestRenderChainsCap(t *testing.T) {
	cs := []statusdata.Chain{
		{Label: "old chain", Steps: []string{"a", "b", "c"}},
		{Label: "new chain", Steps: []string{"x", "y", "z"}},
	}
	out, err := render(statusdata.StatusData{Chains: cs}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, "⛓"); n != maxChains {
		t.Errorf("got %d chains rendered, want %d (cap)", n, maxChains)
	}
	if !strings.Contains(out, "new chain") {
		t.Error("newest chain missing -- cap should keep the newest, not the oldest")
	}
	if strings.Contains(out, "old chain") {
		t.Error("oldest chain present -- cap should have dropped it")
	}
}

func TestRenderSparklineGating(t *testing.T) {
	cs := []statusdata.Contribution{{Decision: "d", Pushing: []string{"p"}, Constraining: []string{"c"}, Durability: "durable"}}

	out, err := render(statusdata.StatusData{Contributions: cs}, trendSeries{ratio: make([]float64, minSparkPoints-1)}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "provisional trend") {
		t.Error("below minSparkPoints: sparkline should not render")
	}

	out, err = render(statusdata.StatusData{Contributions: cs}, trendSeries{ratio: make([]float64, minSparkPoints)}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "provisional trend") {
		t.Error("at minSparkPoints: sparkline should render")
	}
}

func TestRenderSparklineShowsLatestValue(t *testing.T) {
	cs := []statusdata.Contribution{{Decision: "d", Pushing: []string{"p"}, Constraining: []string{"c"}, Durability: "durable"}}
	history := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.73}
	out, err := render(statusdata.StatusData{Contributions: cs}, trendSeries{ratio: history}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "73%") {
		t.Error("sparkline row should show the latest (last) history value as a percentage, not just the shape")
	}
	if strings.Contains(out, "10%") {
		t.Error("sparkline row showed the oldest value, not the latest")
	}
}

func TestRenderCallsVelocityGating(t *testing.T) {
	cs := []statusdata.Consequence{{Next: "n", LeadsTo: "l"}}

	out, err := render(statusdata.StatusData{Consequences: cs}, trendSeries{calls: make([]float64, minSparkPoints-1)}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "tool-call velocity") {
		t.Error("below minSparkPoints: velocity sparkline should not render")
	}

	out, err = render(statusdata.StatusData{Consequences: cs}, trendSeries{calls: make([]float64, minSparkPoints)}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "tool-call velocity") {
		t.Error("at minSparkPoints: velocity sparkline should render")
	}
}

func TestRenderCallsVelocityShowsLatestCountSingularAndPlural(t *testing.T) {
	cs := []statusdata.Consequence{{Next: "n", LeadsTo: "l"}}

	out, err := render(statusdata.StatusData{Consequences: cs}, trendSeries{calls: []float64{0, 2, 4, 6, 3, 1}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1 call") || strings.Contains(out, "1 calls") {
		t.Errorf("latest value 1 should read \"1 call\", not \"1 calls\" or the older \"3\": %s", out)
	}

	out, err = render(statusdata.StatusData{Consequences: cs}, trendSeries{calls: []float64{0, 2, 4, 6, 1, 8}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "8 calls") {
		t.Errorf("latest value 8 should read \"8 calls\": %s", out)
	}
}

func TestRenderCallsVelocityOrphanedWithoutConsequencesIsNotShown(t *testing.T) {
	// Nested inside Consequences' own visibility, same as the provisional
	// trend is nested inside Contributions' -- a trend about a section
	// that isn't currently rendering anything reads as orphaned, not
	// informative, even if the historical data still exists.
	out, err := render(statusdata.StatusData{Context: []string{"x"}}, trendSeries{calls: make([]float64, minSparkPoints)}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "tool-call velocity") {
		t.Error("no Consequences currently shown: velocity sparkline should not render")
	}
}

func TestSyncToolCountNewGoalResets(t *testing.T) {
	cur := toolCount{Goal: "old goal", Count: 14, Since: 1000}
	tc, changed := syncToolCount(cur, "new goal", 2000)
	if !changed {
		t.Fatal("differing goal: want changed=true")
	}
	if tc != (toolCount{Goal: "new goal", Count: 0, Since: 2000}) {
		t.Errorf("syncToolCount = %+v, want {new goal, 0, 2000}", tc)
	}
}

func TestSyncToolCountSameGoalPreserved(t *testing.T) {
	cur := toolCount{Goal: "same goal", Count: 14, Since: 1000}
	tc, changed := syncToolCount(cur, "same goal", 9999)
	if changed {
		t.Fatal("matching goal: want changed=false")
	}
	if tc != cur {
		t.Errorf("syncToolCount = %+v, want cur returned unchanged (%+v)", tc, cur)
	}
}

func TestSyncToolCountMissingPriorGoalTreatedAsNew(t *testing.T) {
	// The zero value (what readToolCount returns for a missing or
	// malformed sidecar) has Goal == "" -- that must not accidentally
	// match a real goal and skip the reset.
	tc, changed := syncToolCount(toolCount{}, "first real goal", 500)
	if !changed {
		t.Fatal("zero-value cur: want changed=true (no prior goal to match)")
	}
	if tc != (toolCount{Goal: "first real goal", Count: 0, Since: 500}) {
		t.Errorf("syncToolCount = %+v, want {first real goal, 0, 500}", tc)
	}
}

func TestExpireStaleConsequenceDropsPastThreshold(t *testing.T) {
	d := statusdata.StatusData{
		Consequences: []statusdata.Consequence{
			{Next: "an old, still-pending goal", LeadsTo: "x"},
		},
	}
	tc := toolCount{Goal: "an old, still-pending goal", Count: expireAfterCalls + 1, Since: 0}
	got := expireStaleConsequence(d, tc)
	if len(got.Consequences) != 0 {
		t.Errorf("got %d Consequences, want 0 -- a goal past expireAfterCalls should be dropped", len(got.Consequences))
	}
}

func TestExpireStaleConsequenceKeepsFreshGoal(t *testing.T) {
	d := statusdata.StatusData{
		Consequences: []statusdata.Consequence{
			{Next: "a fresh goal", LeadsTo: "x"},
		},
	}
	tc := toolCount{Goal: "a fresh goal", Count: expireAfterCalls, Since: 0}
	got := expireStaleConsequence(d, tc)
	if len(got.Consequences) != 1 {
		t.Errorf("got %d Consequences, want 1 -- exactly at the threshold should not expire yet", len(got.Consequences))
	}
}

func TestExpireStaleConsequenceIgnoresTallyForADifferentGoal(t *testing.T) {
	// tc tracking a stale, already-superseded goal must never be applied
	// to the CURRENT newest Consequence just because it's also over the
	// threshold -- that would expire fresh content off a count that was
	// never actually counting it.
	d := statusdata.StatusData{
		Consequences: []statusdata.Consequence{
			{Next: "a brand new goal", LeadsTo: "x"},
		},
	}
	tc := toolCount{Goal: "a different, older goal", Count: expireAfterCalls + 50, Since: 0}
	got := expireStaleConsequence(d, tc)
	if len(got.Consequences) != 1 {
		t.Error("a tally for a different goal should never expire the current one")
	}
}

func TestExpireStaleConsequenceNoopOnEmpty(t *testing.T) {
	got := expireStaleConsequence(statusdata.StatusData{}, toolCount{Count: 999})
	if len(got.Consequences) != 0 {
		t.Error("no Consequences to begin with should stay that way")
	}
}

func TestReadToolCountMissingFile(t *testing.T) {
	dir := t.TempDir()
	tc := readToolCount(filepath.Join(dir, "does-not-exist.json"))
	if tc != (toolCount{}) {
		t.Errorf("readToolCount on missing file = %+v, want the zero value", tc)
	}
}

func TestReadToolCountMalformedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".pellicle-tool-count.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	tc := readToolCount(path)
	if tc != (toolCount{}) {
		t.Errorf("readToolCount on malformed file = %+v, want the zero value (same as missing)", tc)
	}
}

func TestWriteReadToolCountRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".pellicle-tool-count.json")
	want := toolCount{Goal: "ship it", Count: 7, Since: 42}
	if err := writeToolCount(path, want); err != nil {
		t.Fatal(err)
	}
	if got := readToolCount(path); got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestLoadToolTallyNewGoalResetsAndPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".pellicle-tool-count.json")
	if err := writeToolCount(path, toolCount{Goal: "old goal", Count: 9, Since: 100}); err != nil {
		t.Fatal(err)
	}
	tc, err := loadToolTally(path, "new goal", 500)
	if err != nil {
		t.Fatal(err)
	}
	if tc != (toolCount{Goal: "new goal", Count: 0, Since: 500}) {
		t.Errorf("loadToolTally = %+v, want reset to {new goal, 0, 500}", tc)
	}
	// The reset has to actually land on disk, not just be returned --
	// the NEXT render (and every tool call in between, via
	// status-tool-count.sh) needs to see it too.
	if onDisk := readToolCount(path); onDisk != tc {
		t.Errorf("on-disk state after reset = %+v, want it to match the returned tally %+v", onDisk, tc)
	}
}

func TestLoadToolTallySameGoalPreservesCountAndDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".pellicle-tool-count.json")
	if err := writeToolCount(path, toolCount{Goal: "steady goal", Count: 14, Since: 100}); err != nil {
		t.Fatal(err)
	}
	// Make the file itself read-only so any write attempt (not just a
	// changed one) would fail -- proves the same-goal branch genuinely
	// skips the write rather than happening to round-trip identical
	// bytes. Incrementing count is status-tool-count.sh's job on the
	// next tool call, not this function's.
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o600) })

	tc, err := loadToolTally(path, "steady goal", 9999)
	if err != nil {
		t.Fatalf("loadToolTally on a matching goal attempted a write: %v", err)
	}
	if tc.Count != 14 || tc.Since != 100 {
		t.Errorf("loadToolTally = %+v, want the existing count/since preserved (14, 100)", tc)
	}
}

func TestLoadToolTallyThenRenderEndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".pellicle-tool-count.json")
	if err := writeToolCount(path, toolCount{Goal: "ship the tally", Count: 14, Since: 1000}); err != nil {
		t.Fatal(err)
	}
	now := int64(1000 + 23*60 + 7) // same goal, 23m7s later
	tc, err := loadToolTally(path, "ship the tally", now)
	if err != nil {
		t.Fatal(err)
	}
	tally := formatToolTally(tc.Count, now-tc.Since)
	out, err := render(statusdata.StatusData{Consequences: []statusdata.Consequence{{Next: "ship the tally", LeadsTo: "l"}}}, trendSeries{}, tally)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "14 calls, 23m") {
		t.Errorf("end-to-end tally missing from render output; got:\n%s", out)
	}
}

func TestFormatDurationSubMinuteReadsAsSeconds(t *testing.T) {
	cases := []struct {
		seconds int64
		want    string
	}{
		{0, "0s"},
		{1, "1s"},
		{59, "59s"},
	}
	for _, c := range cases {
		if got := formatDuration(c.seconds); got != c.want {
			t.Errorf("formatDuration(%d) = %q, want %q", c.seconds, got, c.want)
		}
	}
}

func TestFormatDurationMinutesAndAbove(t *testing.T) {
	cases := []struct {
		seconds int64
		want    string
	}{
		{60, "1m"},
		{119, "1m"}, // whole minutes only, no fractional/rounded-up minute
		{1380, "23m"},
	}
	for _, c := range cases {
		if got := formatDuration(c.seconds); got != c.want {
			t.Errorf("formatDuration(%d) = %q, want %q", c.seconds, got, c.want)
		}
	}
}

func TestFormatToolTallySingularVsPluralCalls(t *testing.T) {
	one := formatToolTally(1, 5)
	if !strings.Contains(one, "1 call,") || strings.Contains(one, "1 calls,") {
		t.Errorf("formatToolTally(1, ...) = %q, want singular \"1 call,\"", one)
	}
	many := formatToolTally(14, 5)
	if !strings.Contains(many, "14 calls,") {
		t.Errorf("formatToolTally(14, ...) = %q, want \"14 calls,\"", many)
	}
}

func TestFormatToolTallyFreshlyResetGoalReadsZeroCalls(t *testing.T) {
	got := formatToolTally(0, 0)
	if !strings.Contains(got, "0 calls, 0s") {
		t.Errorf("formatToolTally(0, 0) = %q, want it to contain \"0 calls, 0s\" for a just-reset goal", got)
	}
}

func TestRenderAppendsToolTallyToLastConsequenceOnly(t *testing.T) {
	cs := []statusdata.Consequence{
		{Next: "first next", LeadsTo: "l1"},
		{Next: "second next", LeadsTo: "l2"},
	}
	out, err := render(statusdata.StatusData{Consequences: cs}, trendSeries{}, "  — 14 calls, 23m")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "☐ second next  — 14 calls, 23m") {
		t.Errorf("tally missing or misplaced on the newest Consequence; got:\n%s", out)
	}
	if strings.Contains(out, "first next  — 14 calls, 23m") {
		t.Error("tally should not repeat on an older Consequence still shown under the cap")
	}
}

func TestRenderReservesRoomForToolTallyWhenTruncatingNext(t *testing.T) {
	// Regression, 2026-08-22: a word-count reservation doesn't help when
	// the overflow is character density, not word count -- the real case
	// that motivated this fix was a 16-word Next field full of long
	// identifiers ("drivers/constraints", "headlineLines", "CHANGELOG")
	// that sat entirely under a word cap while still running long enough
	// in characters that appending the tally pushed the line to 162
	// visible chars against every other line's own 140-char cap. This
	// reproduces that shape directly: real words, real length, no tally
	// accounting needed until one is actually appended.
	next := "commit wiring drivers/constraints into -headline (headlineLines reorder, tests, README, CHANGELOG), then make install and git push"

	withoutTally, err := render(statusdata.StatusData{Consequences: []statusdata.Consequence{{Next: next, LeadsTo: "l"}}}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	withTally, err := render(statusdata.StatusData{Consequences: []statusdata.Consequence{{Next: next, LeadsTo: "l"}}}, trendSeries{}, "  — 50 calls, 566m")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(withoutTally, "git push") {
		t.Errorf("without a tally, expected next rendered in full (it's under both caps); got:\n%s", withoutTally)
	}
	for _, line := range strings.Split(withTally, "\n") {
		if !strings.HasPrefix(line, "- ☐ ") {
			continue
		}
		if l := utf8.RuneCountInString(line); l > maxLineChars+len("- ☐ ")+30 {
			t.Errorf("next+tally line ran to %d visible chars, want it bounded near maxLineChars; got:\n%s", l, line)
		}
	}
	if strings.Contains(withTally, "git push  — 50 calls, 566m") {
		t.Error("with a tally present, next should be truncated shorter than the full text -- the tally shouldn't just tack onto an unshortened line")
	}
}

func TestRenderOmitsToolTallyWhenEmpty(t *testing.T) {
	cs := []statusdata.Consequence{{Next: "only next", LeadsTo: "l"}}
	out, err := render(statusdata.StatusData{Consequences: cs}, trendSeries{}, "")
	if err != nil {
		t.Fatal(err)
	}
	// "—" alone isn't a safe check anymore -- the header line ("### pellicle
	// — updated ...") always carries one. The tally's own signature is the
	// call count.
	if strings.Contains(out, "calls,") || strings.Contains(out, "call,") {
		t.Error("empty toolTally should add nothing to the rendered output")
	}
}

func TestSanitizeTextStripsEscapeSequences(t *testing.T) {
	in := "before\x1b[31mred\x1b]52;c;ZXZpbA==\x07after"
	got := sanitizeText(in)
	if strings.ContainsAny(got, "\x1b\x07") {
		t.Errorf("control bytes survived sanitizeText: %q", got)
	}
	want := "before[31mred]52;c;ZXZpbA==after"
	if got != want {
		t.Errorf("sanitizeText(%q) = %q, want %q", in, got, want)
	}
}

func TestSanitizeTextPreservesOrdinaryUnicodeText(t *testing.T) {
	in := "a plain line with émoji 🎯 and 日本語"
	if got := sanitizeText(in); got != in {
		t.Errorf("sanitizeText should not touch non-control text: got %q, want %q", got, in)
	}
}

func TestSanitizeStatusDataCoversEveryRenderedField(t *testing.T) {
	esc := "poison\x1b[2Jtext"
	d := statusdata.StatusData{
		Drivers:     []string{esc},
		Constraints: []string{esc},
		Context:     []string{esc},
		Chains: []statusdata.Chain{{
			Label: esc,
			Steps: []string{esc, esc, esc},
		}},
		Contributions: []statusdata.Contribution{{
			Decision: esc, Pushing: []string{esc}, Constraining: []string{esc}, Durability: "durable",
		}},
		Consequences: []statusdata.Consequence{{Next: esc, LeadsTo: esc}},
	}
	got := sanitizeStatusData(d)
	fields := []string{
		got.Drivers[0], got.Constraints[0],
		got.Context[0], got.Chains[0].Label, got.Chains[0].Steps[0],
		got.Contributions[0].Decision, got.Contributions[0].Pushing[0], got.Contributions[0].Constraining[0],
		got.Consequences[0].Next, got.Consequences[0].LeadsTo,
	}
	for i, f := range fields {
		if strings.ContainsRune(f, 0x1b) {
			t.Errorf("field %d still contains ESC after sanitizeStatusData: %q", i, f)
		}
	}
}

func TestCapStatusDataKeepsNewestPerSection(t *testing.T) {
	seq := func(n int) []string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, fmt.Sprintf("item %d", i))
		}
		return out
	}
	d := statusdata.StatusData{
		Drivers:     seq(maxStandingFactors + 2),
		Constraints: seq(maxStandingFactors + 2),
		Context:     seq(maxContextLines + 2),
		Chains:      []statusdata.Chain{{Label: "old"}, {Label: "new"}},
		Contributions: []statusdata.Contribution{
			{Decision: "d0"}, {Decision: "d1"}, {Decision: "d2"}, {Decision: "d3"},
		},
		Consequences: []statusdata.Consequence{
			{Next: "n0"}, {Next: "n1"}, {Next: "n2"},
		},
	}
	got := capStatusData(d)

	if n := len(got.Drivers); n != maxStandingFactors {
		t.Errorf("Drivers has %d entries, want %d", n, maxStandingFactors)
	}
	if want := fmt.Sprintf("item %d", maxStandingFactors+1); got.Drivers[len(got.Drivers)-1] != want {
		t.Errorf("Drivers dropped the newest entry, got last=%q want %q", got.Drivers[len(got.Drivers)-1], want)
	}
	if n := len(got.Constraints); n != maxStandingFactors {
		t.Errorf("Constraints has %d entries, want %d", n, maxStandingFactors)
	}
	if n := len(got.Context); n != maxContextLines {
		t.Errorf("Context has %d entries, want %d", n, maxContextLines)
	}
	if n := len(got.Chains); n != maxChains {
		t.Errorf("Chains has %d entries, want %d", n, maxChains)
	}
	if got.Chains[0].Label != "new" {
		t.Errorf("Chains kept the oldest entry, got %q want %q", got.Chains[0].Label, "new")
	}
	if n := len(got.Contributions); n != maxContributions {
		t.Errorf("Contributions has %d entries, want %d", n, maxContributions)
	}
	if last := got.Contributions[len(got.Contributions)-1].Decision; last != "d3" {
		t.Errorf("Contributions dropped the newest entry, got last=%q want %q", last, "d3")
	}
	if n := len(got.Consequences); n != maxConsequences {
		t.Errorf("Consequences has %d entries, want %d", n, maxConsequences)
	}
	if last := got.Consequences[len(got.Consequences)-1].Next; last != "n2" {
		t.Errorf("Consequences dropped the newest entry, got last=%q want %q", last, "n2")
	}
}

func TestCapStatusDataLeavesShortDataUntouched(t *testing.T) {
	d := statusdata.StatusData{
		Drivers:       []string{"one driver"},
		Context:       []string{"one context line"},
		Contributions: []statusdata.Contribution{{Decision: "only decision"}},
		Consequences:  []statusdata.Consequence{{Next: "only next"}},
	}
	got := capStatusData(d)
	if len(got.Drivers) != 1 || len(got.Context) != 1 || len(got.Contributions) != 1 || len(got.Consequences) != 1 {
		t.Errorf("capStatusData shortened data that was already under every cap: %+v", got)
	}
}

func TestMarshalPrunedInputKeepsPunctuationLiteral(t *testing.T) {
	d := statusdata.StatusData{
		Context: []string{"a -> b, and words<=N, and a & b"},
	}
	got, err := marshalPrunedInput(d)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, esc := range []string{"\\u003c", "\\u003e", "\\u0026"} {
		if strings.Contains(s, esc) {
			t.Errorf("marshalPrunedInput HTML-escaped ordinary punctuation (found %s): %s", esc, s)
		}
	}
	if !strings.Contains(s, "a -> b, and words<=N, and a & b") {
		t.Errorf("marshalPrunedInput did not preserve the literal punctuation: %s", s)
	}
	var roundTrip statusdata.StatusData
	if err := json.Unmarshal(got, &roundTrip); err != nil {
		t.Fatalf("marshalPrunedInput produced invalid JSON: %v", err)
	}
	if roundTrip.Context[0] != d.Context[0] {
		t.Errorf("round trip changed content: got %q, want %q", roundTrip.Context[0], d.Context[0])
	}
}

func TestJoinTruncatedCapsAtMaxItems(t *testing.T) {
	var items []string
	for i := 0; i < 5; i++ {
		items = append(items, fmt.Sprintf("f%d", i))
	}
	got := joinTruncated(items, 3, 20)
	want := "f0; f1; f2"
	if got != want {
		t.Errorf("joinTruncated = %q, want %q", got, want)
	}
}

func TestJoinTruncatedLeavesShortItemsUntouched(t *testing.T) {
	items := []string{"a short one", "another short one"}
	if got := joinTruncated(items, 3, 20); got != "a short one; another short one" {
		t.Errorf("joinTruncated(%v) = %q, want %q", items, got, "a short one; another short one")
	}
}

func TestJoinTruncatedCutsEachItemIndividuallyNotTheJoinedBlob(t *testing.T) {
	// The bug this replaces: joining first, then truncating the whole
	// joined string to one shared word budget, let a long first item
	// consume the entire budget and silently destroy every item after it
	// -- found live, 2026-08-22, against this project's own real
	// status-input.json, where a genuinely load-bearing second
	// constraining factor got cut to three words and vanished. Per-item
	// truncation means a later, shorter item survives intact regardless
	// of how long an earlier sibling runs.
	long := strings.Repeat("word ", 30) + "end"
	items := []string{long, "short survivor"}
	got := joinTruncated(items, 2, 10)
	if !strings.Contains(got, "short survivor") {
		t.Errorf("joinTruncated(%v) = %q, second item should survive intact", items, got)
	}
}

func TestTruncateWordsBackstopsUnspacedText(t *testing.T) {
	// A single whitespace-free run (CJK prose doesn't use inter-word
	// spaces the way Latin scripts do; a long URL is the Latin
	// equivalent) is one "word" by strings.Fields' definition, so the
	// word-count cap alone never fires on it.
	long := strings.Repeat("日", 300)
	got := truncateWords(long, maxLineWords)
	if n := utf8.RuneCountInString(got); n > maxLineChars {
		t.Errorf("truncateWords left %d runes, want <= %d", n, maxLineChars)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated unspaced text should end with an ellipsis, got %q", got)
	}
}

func TestTruncateWordsLeavesShortTextUntouched(t *testing.T) {
	in := "a short line well under both caps"
	if got := truncateWords(in, maxLineWords); got != in {
		t.Errorf("truncateWords(%q) = %q, want unchanged", in, got)
	}
}

func TestTruncateWordsEscapesMarkdownStructuralCharacters(t *testing.T) {
	// An unmatched backtick or a run of asterisks/underscores in
	// Claude-authored free text can open a code span or emphasis run that
	// swallows everything after it once this lands in a real markdown
	// transcript -- these three characters must come out escaped.
	in := "run `render-status` with *any* flag, e.g. --dry_run"
	got := truncateWords(in, maxLineWords)
	want := "run \\`render-status\\` with \\*any\\* flag, e.g. --dry\\_run"
	if got != want {
		t.Errorf("truncateWords(%q) = %q, want %q", in, got, want)
	}
}
