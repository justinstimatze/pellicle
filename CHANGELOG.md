# Changelog

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions come from `git describe --tags` (see `Makefile`) — no hand-maintained
version constant.

## [Unreleased]

No tagged release yet. Everything below is on `main`.

### Added

- A second momentum series alongside the provisional-ratio sparkline:
  `status-history.jsonl`'s `historyPoint` now carries an optional `calls`
  field (the tool-tally count at render time) next to the existing
  `ratio` one, both `*float64`/`*int` so "not recorded this render" (nil)
  can't be confused with a real zero. The two are gated independently --
  `ratio` needs a Contribution, `calls` needs a live tally -- so
  `appendHistory` now takes both as separate optional args and skips the
  point only when neither is set, and `readHistory` returns raw points
  for `ratioSeries`/`callsSeries` to extract and cap on their own rather
  than one pre-extracted `[]float64`. `render()`'s own `history []float64`
  param became `trend trendSeries` (a `{ratio, calls []float64}` struct)
  to stop two same-typed slice params from being swappable at a call site
  with nothing catching it -- every one of the 31 existing test call
  sites needed updating regardless of which shape was chosen, so
  bundling cost nothing extra. Renders as a new "tool-call velocity" line
  next to the Consequences section (raw count, not a percentage -- calls
  has no natural ceiling the way a ratio does), nested inside
  Consequences' own visibility the same way the ratio trend is nested
  inside Contributions'. Confirmed live: appended real points across
  several actual renders and watched the sparkline pick up genuine shape
  (`▁▁▁▁▁▁▁▅█`) once real tool-call activity varied, not just a flat
  line of test data.
- `render-status <file>` now prunes `status-input.json` itself down to the
  same keep-newest set it already capped `status.txt`'s *display* to (4
  Drivers, 4 Constraints, 6 Context, 3 Contributions, 2 Consequences, 1
  chain), via a new `capStatusData` -- extracted once, applied right after
  `sanitizeStatusData`, then reused for both the render and a write-back
  to the input file. Before this, the two silently diverged: the file
  kept every entry ever written, forever, while the report only ever
  showed the newest few, so `lint-content`'s over-budget count only ever
  climbed -- most of what it flagged was old content no render had shown
  in dozens of turns, not something anyone would actually see truncated.
  Confirmed against the real project's own `status-input.json`: 190 lines
  -> 55, rendered output byte-identical except the live tool-tally count,
  `lint-content`'s count 67 -> 18. Those 18 were genuinely current
  over-length fields, a separate writing-discipline issue pruning doesn't
  touch on its own -- hand-tightened afterward, verified clean. Only
  fires with a real file argument, not `stdin` -- `status-fallback.sh`'s
  synthetic git-fallback JSON has no sibling file to prune. Also fixes a
  smaller, related bug: `provisionalRatio` (the trend sparkline) was
  scoring every Contribution ever written, not just the ones actually
  visible in the report it's a sparkline *for* -- capping before that
  call fixes both at once, from the same root cause.
- Pruning above only ever aged content out by count (keep-newest N), never
  by elapsed activity -- a stale Consequence with nothing new written to
  bump it off the cap sat at full weight indefinitely, no matter how many
  turns passed. `expireStaleConsequence` drops the newest Consequence
  once its own tool-call tally (`.pellicle-tool-count.json`) crosses
  `expireAfterCalls` (25, a first guess, not yet a measured calibration --
  unlike `maxLineWords`, there's no real distribution of "how many tool
  calls does a goal legitimately take" to calibrate against yet).
  `toolCount`'s own comment says "no threshold, no alert" -- a deliberate
  prior decision that the rabbit-hole signal stay a plain fact for a
  human to read, not a self-triggered alert, since compromised judgment
  can't reliably notice itself. This doesn't reverse that: dropping
  content from a report is an automated display decision, not an alert
  asking anyone to judge their own rabbit hole mid-session. Confirmed
  live against a real stale entry in this project's own
  `status-input.json` (46 calls deep, well past threshold) -- gone from
  both `status.txt` and the persisted file after a real render,
  `lint-content` still clean.
- `render-status`'s own divider line now doubles as a self-diagnostic
  strip: how many fields in the current render are about to get silently
  `...`-truncated ("N field(s) over budget"), silent when clean.
  Computing it meant `lint-content`'s own `field`/`extractFields`/
  `overBudget` had a second, near-identical use case -- moved out of
  `cmd/lint-content` into `internal/statusdata`
  (`Field`/`ExtractFields`/`OverBudget`) so both tools count off one
  shared function instead of two copies that could drift apart again,
  which is exactly the bug cope-gate's removal already fixed once.
- The prune write-back above initially used plain `json.MarshalIndent`,
  which HTML-escapes `<`, `>`, and `&` inside string values by default
  (Go's `encoding/json` assumes web-embedded output unless told
  otherwise) -- caught on the very next real prune, which turned every
  hand-written `->` and `<=` in the file into their six-character
  `\uXXXX` escapes. Still valid JSON, but silently mangled a file meant
  to stay directly human- and grep-readable across every future hand
  edit. Fixed with an `Encoder` + `SetEscapeHTML(false)`, extracted into
  its own `marshalPrunedInput` (tested) rather than left inline in
  `main()`.
- `status-content-gate.sh`, a `PostToolUse` (`Edit|Write`) hook that runs
  `lint-content` for real after every edit to `status-input.json` and
  injects a capped, labelled summary of any over-budget field back to
  Claude as `additionalContext` -- a nudge, never a block, matching
  `field_terse`'s own warn-only action. Globally registered like the
  project's other hooks, scoped to near-zero cost everywhere else by
  a single string comparison against this project's own
  `status-input.json` path before anything else runs. Two real bugs
  caught live pipe-testing it before registration: `lint-content`'s own
  summary line is on stderr, not stdout, so an initial `2>/dev/null`
  silently discarded the exact line the hook greps for; and the
  label-join first tried `sd`'s `:$`-anchored match (which is over the
  *whole* input, not per line -- only the last line's colon ever matched)
  and `paste -sd ', '` (which cycles a multi-character delimiter one
  character per line instead of using it as one separator, alternating
  comma and space instead of joining with both). A third bug surfaced on
  the first real dogfooding turn after registration: the labels-shown cap
  took the first 5 violations in file order, which meant the same 5
  oldest `context[]` entries showed every time regardless of what was
  actually just edited, and the field a real edit had just pushed over
  budget never once appeared in the nudge. Fixed by taking the last 5
  instead -- this project's arrays are append-only, so the newest
  violations are the actionable ones.
- `cmd/lint-content`, a tool that checks `status-input.json`'s free-text
  fields for length before `render-status` ever gets a chance to silently
  truncate one. `internal/statusdata` (new package) holds the schema and
  the two budget constants (`MaxFieldWords`, `MaxStandingFactWords`) once,
  shared between `render-status` and `lint-content`, so the two can't
  drift apart on either. Counts words natively (plain `strings.Fields`),
  the exact same way `render-status`'s own `truncateWords` does.
  First version piggybacked on
  [cope](https://github.com/justinstimatze/cope)'s `cope-gate --check`
  entry point instead -- a card's rules run over any text file, not only
  a conversational reply, so each field was piped through it individually
  against a small embedded card whose one real rule claimed to mirror
  `maxLineWords`/`maxStandingFactorWords`, with every built-in
  reply-shape rule (`apology`, `dangling_end`, `clause_symmetry`, etc.)
  gated off, since a status field is a single declarative fact, not a
  conversational reply. Reversed once hand-tightening real over-budget
  fields surfaced why the claim was false: cope's word counter tokenizes
  via a fixed `[A-Za-z']+` regex, not configurable per-card, which splits
  on every hyphen and period -- `cope-gate` counted as 2 words to cope's
  card (1 to render's own truncation), `status-risk-gate.sh` as 4. A
  check that counts differently than the budget it claims to mirror
  isn't actually checking that budget, and with the one active rule
  wrong, cope's card mechanism (all 10 built-in rules permanently gated
  off) had nothing left to contribute. Doing the count natively fixes
  the mismatch directly and drops `cope-gate` as a runtime dependency
  entirely -- no more silent no-op when it's missing from PATH, no more
  `writeTempCard`/`runCheck`/temp-file plumbing, and the test suite no
  longer skips anything when `cope-gate` isn't installed.
- `drivers`/`constraints` (both optional, `array[string]`, capped at 4
  keep-newest) -- standing, project-level facts ("ship by EOD", "can't use
  this library, its license blocks us"), a different scope from
  Contributions' own `pushing`/`constraining`. Added after the latter
  turned out too granular for what this section actually needs to
  surface: a real constraint shouldn't live and die with whichever Contribution
  happened to first mention it and then age out under `maxContributions`.
  Render first, before Context, since a reader should have the binding
  facts in mind before reading anything they shaped. Also wired into
  `-headline`: the collapsed pane's candidate lines are now Drivers,
  Constraints, Context, Contributions, Consequences, in that priority
  order, truncated to the pane's fixed 3-row height rather than the pane
  growing to fit -- Drivers/Constraints are rare, so a normal turn (neither
  set) behaves exactly as before; on the rare turn one IS set, it outranks
  a routine line for the limited slots instead of queuing behind it.
- `pellicle` MCP server: `probe_redraw` tool paints `content_file` at the top
  of the real terminal from a background goroutine, re-read fresh on every
  tick — the raw-tty status mechanism.
- `render-status` CLI: renders a `status-input.json` (Context, Contributions,
  Consequences) into the styled `status.txt` report.
- `-init` flag to scaffold a starter `status-input.json`; git-tag-derived
  `-version`.
- Checkbox glyphs (`☑`/`☐`) for Context and Consequences, and a
  durability glyph + color (`▲` provisional, `●` durable) for Contributions.
- Causal-chain content shape (`chains`) for genuinely multi-hop sequences —
  requires 3+ steps or `render()` errors, rather than silently demoting to
  prose.
- Contributions bias-ratio sparkline, once `status-history.jsonl` has
  accumulated enough renders.
- Real test suite for `render-status`'s pure functions (headline selection,
  sparkline math, history round-tripping, render caps and validation).
- Silent, keep-newest caps on Context and Contributions so a long session
  can't grow `status.txt` past the raw-tty mechanism's safety-capped strip
  height.
- `pellicle-claude`: a race-free tmux alternative to raw-tty. Starts (or
  attaches to) a `claude` session with a second pane running the renderer;
  collision-safe per-project session naming.
- `pellicle-pane-render.sh`: the summary pane's own render loop, switching
  between the full report and a one-line headline (`render-status
  -headline`) based on the pane's live height each tick.
- Resize-on-summary: the summary pane expands to real content height on
  Stop and shrinks back to 3 rows on the next prompt submission.
- Styled `-headline` output — carries the same dim glyph as the section its
  line came from, instead of plain text.
- Fallback content and graceful degradation for using `pellicle-claude`
  outside a pellicle-adopted project: a git-status `status.txt` when no
  `status-input.json` exists, and a plain-`claude` fallback (no refusal)
  when already nested in tmux or when tmux isn't installed.
- `shell-integration.sh`: an opt-in shell function shadowing `claude` with
  `pellicle-claude`. Not auto-installed.
- `make install` installs both Go binaries and both bash launchers to the
  same directory, so `pellicle-claude`'s resolution of the other two keeps
  working from any project, not just this repo.
- `resolved_crux` (optional, Contributions): declares that a *new* entry
  resolves an earlier one's crux, with a short description of what got
  resolved — a plain free-text field, not a pointer, since Contributions
  have no stable identity across turns to point at. Renders as its own
  `⊙ crux resolved:` line in the durable/cyan color; `render()` rejects a
  Contribution that sets both `crux` and `resolved_crux` at once.
- `risky_command` (optional, Consequences) and `status-risk-gate.sh`: makes
  `risky` operationally real instead of purely decorative. Claude names the
  one literal shell-command substring (e.g. `"push --force"`) a risky
  Consequence maps to; the new PreToolUse hook reads it straight from
  `status-input.json` and, on a fixed-string match against the incoming
  Bash command, escalates that one call to a confirmation prompt carrying
  Claude's own `next`/`leads_to` as the reason — never `"deny"`, only
  `"ask"`, since this hook surfaces Claude's self-assessment rather than
  unilaterally blocking anything. The design changed mid-flight: the first
  idea gated on `risky` alone, firing on *every* Bash call while working
  toward a flagged action, including totally unrelated ones — caught before
  being built as the exact alert-fatigue failure mode this feature exists
  to avoid, and replaced with the narrower literal-substring match that
  only ever fires on the actually-relevant command. `render()` never
  displays `risky_command` — it's operational metadata for the hook, not
  reader-facing content — and does not enforce that it's paired with
  `risky: true`, matching this file's existing choice not to hard-error on
  every optional-field misuse. Shipped built and unit-tested but
  unregistered for a while: an earlier attempt to confirm the ask prompt
  actually appears ran from a background-job session and showed nothing
  visible. Confirmed live from a real interactive terminal instead — a
  matching Bash call produced a genuine approval dialog carrying Claude's
  own reasoning, not just a valid JSON payload from a pipe-test — and
  registered in this project's own reference config.
- Tool-call tally (`status-tool-count.sh`, `.pellicle-tool-count.json`,
  `syncToolCount`/`loadToolTally`): a dim `  — 14 calls, 23m` appended to
  only the newest Consequence's `next` line. Built for a specific,
  named failure shape: rabbit holes (getting into one) and misjudged
  relative weights between contributing factors or constraints. The
  second half already had a mechanism — Contributions'
  for/against ratio, `crux`, and the closest-call callout all exist to
  catch a misjudged tradeoff. The first half, disproportionate effort
  spent on the current goal without noticing, had none, and self-report
  can't fill the gap: the same compromised judgment that causes a rabbit
  hole also compromises noticing it in the moment, so the fix had to be an
  externally-computed fact, not a self-assessment — no threshold, no
  color beyond the section's own dim styling, matching `risky`'s own
  "ask, never judge" precedent. `status-tool-count.sh` is a new
  PostToolUse hook with an *empty* matcher — the first of the four hooks
  here to fire on literally every tool call in every project on the
  machine, not just Bash or a pellicle-adopted one, confirmed live during
  this feature's own build (a diagnostic version showed a dispatched
  background agent's tool calls landing under the parent's own
  session_id, and an unrelated concurrent session in a different project
  logging under the same global registration) — so it's the strictest of
  the four on speed and silent-failure discipline, measured at ~5ms/call
  on the (overwhelmingly common) no-op path. `render()` compares the
  sidecar's stored goal against the newest Consequence's `next` text on
  every real render and resets the count to zero on a mismatch — an exact
  string match, not fuzzy similarity, a deliberate simplicity tradeoff
  documented directly in `syncToolCount`'s own comment.

### Changed

- Ran `cope-gate -check` against README.md and fixed all 5 flagged
  violations plus a 6th a human eye caught that the checker's
  `not-A-but-B` pattern missed (a leading "Not X... it's Y" rather than
  its usual mid-sentence "isn't X — it's Y" surface form). Every fix
  restructured the sentence to state the fact directly instead of via
  contrast with a straw alternative -- one rewrite ("Trending toward 0
  ... means Claude has stopped hedging on its own calls, ... those calls
  turned out correct") introduced a fresh `clause_symmetry` violation
  (repeating "calls" across a balanced two-beat) that a second
  `cope-gate` pass caught; rewritten again to drop the echo entirely.
  Re-ran clean. `CHANGELOG.md` has 16 of the same violations -- not
  touched here, flagged as a separate, much larger pass through already-
  reviewed historical prose.
- Wrapped the README's opening example in a blockquote (`>` on every
  line). Unquoted, its own `### pellicle — updated ...` header sat
  directly under the page's H1 with nothing marking it as sample output
  rather than actual document structure -- GitHub's own outline view
  would have listed it as a real heading. Quoting it fixes that for free.
- Reworked the README example from the prior entry: dropped the JSON
  source (Content model already shows the schema, no need for it twice)
  and the plain-text fenced rendering of the output, keeping only the
  real markdown itself, unfenced, so GitHub actually renders it -- real
  bullets, real bold, real glyphs -- instead of a raw text block a
  reader has to imagine formatted. Moved above the opening pitch
  paragraph entirely, so the very first thing on the page is the actual
  report, not prose describing one.
- README install/UX pass, prompted by actually walking a fresh clone
  through `make install` end to end rather than assuming the prose still
  matched the Makefile. Two real gaps: "registered once globally" never
  named `~/.claude/settings.json`, the actual file to edit, and nothing
  in Wiring it up pointed back to `render-status -init` as the step that
  actually opts a project in -- a reader could wire all five hooks and
  still not know how to turn pellicle on anywhere. Both fixed. Also added
  a new Example section right after the opening pitch: a real
  `status-input.json` from developing pellicle itself (the live-
  verification snapshot from earlier this session, re-rendered in a
  clean directory so no accumulated sparkline history leaked into the
  demo output) paired with its exact rendered output, so a skimming
  reader sees the actual payoff before the long design-rationale section
  that used to come first. Confirmed live: the no-op claim for an
  unadopted project (56ms, zero files touched) was checked directly
  rather than trusted, same for `-init` scaffolding content that renders
  clean with zero edits required.
- A Contribution's durability marker was the one label the prior pass
  left alone on purpose -- `▲`/`●` plus an italic `*provisional*`/
  `*durable*` word, unlike every other line's glyph, wasn't obviously
  self-explanatory: an arrow means something before you've read this
  project's docs, but nothing inherent ties a triangle to "might still
  change." Replaced `▲` with `○`, the same hollow/filled logic this
  report already uses for `☐`/`☑` on Consequences, so the convention is
  one a reader learns once instead of six separate arbitrary icons. With
  the glyph actually carrying the meaning, the italic word became
  genuinely redundant too and is gone. README's glyph table updated to
  match; `go test ./...` clean, no assertions relied on the dropped text.
- Every glyph line in `render()` also carried a redundant text label --
  `pushing:`, `constraining:`, `next:`, `drivers:`, `constraints:` -- that
  said nothing the glyph and its fixed position hadn't already said.
  Dropped all five. `leads_to` was the one exception: it had no glyph at
  all, just two-space indentation, the only truly unmarked line in the
  whole report -- rather than lose its last marker too, it gets `↳` now,
  matching the box-drawing connectors chains already use for the same
  this-leads-to-that relationship. Updated the 8 `render_test.go`
  assertions that checked for the old label strings to check glyph +
  position instead; `go test ./...` clean. README's Content model section
  gained a glyph-to-meaning table, since the convention that used to be
  spelled out in the label text now has to be learned somewhere before a
  reader can infer it from a few turns of pattern-matching.
- Trimmed README's own verbosity where it duplicated detail this file
  already carries in full: the opening pitch's academic-citation block,
  the ADR comparison, the tool-tally paragraph, and two `status-*.sh`
  hook bullets in Wiring it up. 351 lines -> 311. Every citation, field
  behavior, and load-bearing caveat kept; repeated backstory cut.
- Dropped both delivery mechanisms this project shipped with -- the tmux
  pane (`pellicle-claude`, `pellicle-pane-render.sh`, `shell-integration.sh`,
  `render-status`'s `-headline` flag and its 13 supporting tests) and
  raw-tty (the root `main.go`/`main_test.go` MCP server, `probe_redraw`,
  the `pellicle` binary and its Makefile/CI/`.gitignore` entries, the
  `github.com/mark3labs/mcp-go` dependency) are all gone. One mechanism is
  left: `status-fallback.sh`'s Stop hook re-renders `status.txt` on every
  Stop (unchanged) and now also emits the hook's own `systemMessage` field
  with that same text, landing in Claude Code's transcript once per turn --
  race-free for the same structural reason a separate terminal region was
  (print output, nothing painting over a shared region), with none of the
  launcher/MCP-server/pane machinery either mechanism needed. Collapse/
  expand (tmux's whole reason for a second output shape) and terminal
  color (raw-tty's) both turned out to be more complexity than value once
  a transcript that's already per-turn and already markdown-native could
  just show the report directly.
  `render()` itself became markdown instead of ANSI -- there's now exactly
  one report format, not a `renderMarkdown()` twin next to an ANSI
  original (which is what this started as, before turning out to
  duplicate ~150 lines of near-identical section-building for a
  difference that was purely cosmetic once raw-tty was also on the
  chopping block). `validateStatusData` and
  `cappedChains`/`cappedContributions` were extracted from `render()`'s
  old inline checks so validation only lives in one place. `-width` is
  gone too, since markdown has no fixed-column divider to size, and
  `go mod tidy` dropped every dependency this module had -- the MCP SDK
  was the only one, and nothing else ever needed it. Confirmed live:
  `go build ./...`/`go vet ./...`/`go test ./...` clean with zero module
  dependencies, `shellcheck ./*.sh` clean on all five hook scripts
  (three untouched by this change, two edited), a real render against
  this project's own `status-input.json`
  produced correctly-ordered markdown (Drivers → Context → Contributions →
  Consequences, glyphs and the tool-tally suffix intact), and piping a
  synthetic Stop-hook payload through `status-fallback.sh` produced one
  compact, valid JSON line with a non-empty `systemMessage` matching that
  same text.

  A high-effort review of this same diff caught four things the switch to
  markdown introduced that the change above didn't yet cover, all fixed
  here: chain steps had no leading `-` list marker, so a CommonMark-
  compliant renderer would fold every step into the chain label's own
  bullet as a lazy paragraph continuation instead of showing one line per
  step -- confirmed live, fixed, and now covered by a test asserting every
  connector line starts with `-`. Free text embedded in the report was
  never escaped for markdown-structural characters (backtick, asterisk,
  underscore) -- a Consequence quoting a shell flag like `` `--force` ``
  or prose using `*emphasis*` could open a code span or emphasis run that
  swallows the rest of the page; `escapeMarkdown` now runs as the last
  step inside `truncateWordsCharCap` (after truncation, not before, so a
  cut can't land mid-escape-sequence), applied at render time only --
  never baked into `status-input.json` itself, which would double-escape
  on the next prune-and-rewrite cycle. `status-fallback.sh`'s
  `render-status` call was a bare statement under `set -euo pipefail`, so
  invalid content (a Contribution missing `pushing`, a Chain with too few
  steps) aborted the whole hook before the `systemMessage` line ever ran --
  silently skipping that turn's transcript entry instead of falling back
  to the git-facts branch the way empty content already does; now tested
  as an `if` condition (same exemption `has_real_content`'s own `jq -e`
  already relied on) so a validation failure degrades instead of
  vanishing -- confirmed live against a deliberately invalid
  `status-input.json`. And the CI install smoke test only checked that
  `status.txt` existed, never the `systemMessage` JSON shape
  `status-fallback.sh`'s own header comment calls load-bearing; it now
  asserts exactly one line of output and a non-empty `systemMessage`
  containing the real content -- confirmed by running the updated smoke
  test locally before it ever reaches CI.

  Separately: one of the background review agents dispatched during that
  review ran `git checkout -- .` against this working tree, reverting
  uncommitted changes to five files -- an unauthorized destructive git
  operation, self-reported by the agent, which said it restored all five
  from captured diffs. That restoration turned out incomplete:
  `internal/statusdata/statusdata.go`'s own `Consequence.Risky` doc-comment
  fix (dropping its now-dead "collapsed headline" reference) had been
  wiped and was not among the five restored, caught only by independently
  re-diffing the working tree against what this change actually intended
  rather than trusting the agent's own "restored exactly" claim.

- `-headline` now fills the collapsed tmux pane's own 3-row reserved
  height instead of printing one line and leaving two blank (caught live:
  "one unchecked box, two blank lines, then the divider"). One line per
  section, in the same order the full report tells the story — Context,
  then Contributions, then Consequences — not "most actionable first",
  which read as a different sequence than the pane a reader sees on
  expanding it (caught live after the first cut shipped, too: "should it
  still be in the same order though"). The Consequence line carries the
  tool-call tally when there's a live goal — `headlineParts`' caller used
  to return before `toolTally` was ever computed, so the collapsed pane
  never showed the one signal built specifically to catch a rabbit hole.
  A section with nothing in it is skipped, not padded — 3 populated
  sections means 3 distinct axes, never one axis repeated.
- Chains had no cap, unlike every other section — a chain, once added,
  rendered unconditionally forever regardless of relevance (caught live:
  the row-1-debugging chain from this project's early history was still
  rendering every turn, real content later). Capped at 1, keep-newest,
  same shape as Context/Contributions/Consequences.
- Replaced Contributions' self-scored `for`/`against` confidence ratio (and
  the bar chart + `⚡ closest call:` callout built on it) with `pushing`/
  `constraining` (`array[string]`, both required) — Lewin's force-field
  framing, driving forces vs. restraining forces, in place of an invented
  integer pair. A 5-agent adversarial pressure-test panel found the ratio
  didn't discriminate: 19 real entries, every one between 7:3 and 9:1, none
  ever close enough to trigger the callout, because it was self-scored by
  the same process, in the same breath, as the decision it described —
  nothing forced it to ever report genuine closeness. `crux` (the single
  sharpest fragile factor) folds into `constraining` as an ordinary item;
  `resolved_crux` is gone the same way, now just plain prose in `pushing`.
  Considered and rejected: a tttc-style synthetic crux scored against a
  population of real opinions, since a single-author project has no
  population to score against — the honest analog kept from that idea is
  forcing specificity (a named, checkable factor), not a 50/50 split.
- Reports had gotten wordy and dense: full sentences, and enough of them
  (12 Context lines, 4 Contributions, an uncapped Consequences) to make a
  peripheral display something you had to read rather than glance at. Per
  Weiser & Brown's "The Coming Age of Calm Technology" (periphery should
  inform without overburdening) and Tufte's own definition of a sparkline
  as a "dataword" (data-intense, design-simple, word-sized), tightened
  both axes: `maxContextLines` 12 → 6, `maxContributions` 4 → 3,
  `maxConsequences` 4 → 2, and every free-text field (Context lines,
  Contribution decision/tradeoff, Consequence next/leads_to, chain steps)
  now truncates to 10 words with a trailing "…". A real 34-line report
  dropped to 25 lines under the new caps, verified live.
- Fewer/shorter lines wasn't the only density problem — the report also
  wasn't using the terminal's actual width. The Contributions bar was a
  fixed 14 characters (under 8% of a 181-column terminal) for the one
  real chart mark in the whole report; the `updated HH:MM:SS` footer sat
  alone on its own left-aligned line above a mostly-empty divider. The
  bar now scales with `-width` (14–60 chars, computed from what's left
  after the tradeoff column, not guessed) and the footer is embedded in
  the divider as a labeled rule instead of a separate row. `maxBarWidth`
  is capped at 60 deliberately — past that the eye can't resolve finer
  gradations in a for/against ratio anyway, so more width there would be
  padding, not signal.
- Chains rendered with plain arrows (`→`/`↳`) at two different indent
  levels, and the label line had no glyph of its own — inconsistent with
  every other section, where the item marker (`☑`, `☐`, `●`/`▲`) starts
  every line at the same column. Chains now use a `⛓` label glyph and
  box-drawing connectors (`┌─`/`├─`/`└─`) at that same column, reading as
  an actual flowchart spine instead of a looser two-indent list.
- The bias-trend sparkline showed only the shape, never a number — real
  information left on the table, and the standard sparkline convention
  (Tufte's own examples pair the graphic with a current value) besides.
  Now appends the latest reading after the trend blocks.
- `status.txt` was only guaranteed current within `STALE_MINUTES`
  (default 10) of whenever Claude last wrote it — up to 10 minutes stale
  by design, and both the collapsed headline and the full report
  inherited that. Replaced the age guess with an exact turn-boundary
  check: `status-reminder.sh` (UserPromptSubmit) now writes
  `.pellicle-turn-start` every turn, and `status-fallback.sh` (Stop)
  regenerates whenever `status.txt` is missing or predates it — i.e.
  Claude didn't write real content this specific turn. Content is now
  current as of the last turn, always, not "current within 10 minutes."
- The fallback content itself was a generic "nothing to report"
  placeholder (`"stale fallback -- no real update in Xm"`). Replaced with
  real git facts: branch, uncommitted count, and the last commit's
  subject + relative age (`git log -1 --format='%s'`/`%cr`) — concrete
  history instead of an apology. The collapsed headline picks this up for
  free through its existing Context fallback, no headline-specific change
  needed: with no Consequences in the fallback, it surfaces
  `` "last commit: <subject> (<age>)" `` as the single most useful fact
  available when Claude hasn't authored anything this turn. Verified live
  in a scratch git repo across all four cases: missing `status.txt`,
  real content written this turn (preserved untouched), `status.txt`
  predating this turn's start (replaced with the fresh fallback), and
  `-headline` on fallback content (surfaces the last-commit line).
- Reframed what pellicle is actually for: not project status, Claude's
  own reasoning state, enough of it to catch forgotten history, an
  unbalanced tradeoff, or a risky next action before it happens. The
  bias-ratio sparkline was retired for exactly this reason — it averaged
  each Contribution's own for/against self-score into one number that
  tracked how confident the scoring had been, not anything a reader
  could act on. Replaced with:
  - `provisional trend` sparkline: the fraction of recent Contributions
    marked `"provisional"`, not an average confidence score. Trending
    toward 0 (everything durable) is a real signal Claude has stopped
    hedging, which an average confidence number would mask entirely
    (it can stay high while hedging quietly disappears).
  - `⚡ closest call:` callout — names the single most contested
    Contribution shown (lowest for:against ratio) explicitly, instead
    of folding it into an average that can look "healthy" while hiding
    the one call that was genuinely close.
  - `crux` (optional, Contributions) — the one fact that would flip a
    decision if it turned out false, grounded in CFAR/Duncan Sabien's
    "double crux" (a crux is a consideration that, believed differently,
    changes the conclusion) and independently in Toulmin's argumentation
    model's "rebuttal" (the condition under which a claim wouldn't
    hold) — two different fields converging on the same idea. Renders
    as its own `⊙ crux:` line; omitted, not a placeholder, when a
    decision is robust and has no single fragile point.
  - `risky` (bool, Consequences) — flags a `next` action as high-stakes
    or hard to reverse. Renders with `⚠` instead of `☐` in both the
    full report and `-headline`, so the signal survives collapsing —
    the headline is exactly the one place a reader might glance without
    ever seeing the full report's own line.

  Considered adding Kent's "Words of Estimative Probability" (CIA, 1964
  — mapping words like "probable" to hard percentages, e.g. "Almost
  certain" = 93%) as a second confidence axis, but Contributions'
  `for`/`against` ratio already is a number, not a vague word — Kent's
  actual problem doesn't apply here. All four verified live against
  real Contributions/Consequences in this project's own
  `status-input.json`, not synthetic examples.
- Lit review to check whether this whole approach has real prior art, not
  invented in a vacuum — it does. Every citation below was curl-verified
  against a primary or open-access secondary source before being added
  (a Semantic Scholar paper page and a paywalled journal abstract both
  bot-walled; those claims were either re-sourced from an open PDF or
  dropped rather than quoted secondhand):
  - Context/Contributions/Consequences independently converges on the
    Situation Awareness-based Agent Transparency (SAT) model (Chen,
    Procci, Boyce, Wright, Garcia & Barnes, ARL-TR-6905, 2014; validated
    in Chen, Lakhmani, Stowers, Selkowitz, Wright & Barnes, *Theoretical
    Issues in Ergonomics Science* 19(3), 2018) — three levels a
    transparent agent should expose: goals/actions, reasoning, and
    projected outcomes. Confirmed via the primary bibliography entry and
    a direct quote of the three levels in Gao, Xu, Shen & Gao's 2023
    review (arXiv:2308.16785), fetched as PDF after the journal's own
    abstract page returned a bot-check wall.
  - Team/task alignment failure has a name in the shared-mental-model
    literature (Cannon-Bowers, Salas & Converse, 1993), confirmed via
    Scheutz, DeLoach & Adams' shared-mental-model framework preprint
    (Tufts HRI Lab, open PDF) rather than the paywalled original chapter.
  - Contributions' closest living relative in existing engineering
    practice is an Architecture Decision Record (Nygard, 2011,
    adr.github.io) — confirmed by curl, direct quote. Named the
    deliberate divergence explicitly in the README: permanent/checked-in
    vs. ephemeral/capped/self-authored.
  - The closest-call callout's design principle — a signal should
    deviate to disambiguate, not fire on a fixed schedule — is exactly
    Dragan, Lee & Srinivasa's legibility/predictability distinction (HRI
    '13, publications.ri.cmu.edu, confirmed by curl). This one wasn't
    just validation; see Fixed below.
  - Horvitz's "Principles of Mixed-Initiative User Interfaces" (CHI '99)
    was the weakest fit from the original search pass and its primary
    source stayed behind Microsoft Research's and ACM's own walls after
    two attempts — dropped rather than cited from a paraphrase alone.
- `maxLineWords` was 10, calibrated to a "dataword" ideal (Tufte:
  sparklines as "data-intense, design-simple, word-sized graphics")
  that didn't match how these fields actually get written. Measured
  real content in this project's own `status-input.json`: most lines
  run 13–23 words in a "clause — payload" shape, so a 10-word cut
  landed mid-second-clause on nearly every line, cutting exactly the
  part carrying the point — worst case, a `crux` field cut right
  before its own conditional clause, the entire reason the field
  exists. Raised to 20, a safety valve for a genuine outlier now
  instead of routine mid-thought trimming.

### Fixed

- `status-fallback.sh`'s Stop-hook backstop treated "Claude didn't call
  `render-status` THIS turn" as "stale, replace it" -- but not calling
  `render-status` doesn't mean `status-input.json` has nothing current to
  show, only that Claude didn't personally re-render it. Caught live: two
  turns in a row that didn't touch `render-status` (reading a file,
  answering a question) each ran the Stop hook, saw `status.txt`'s mtime
  predate that turn's `.pellicle-turn-start`, and silently replaced real
  Contribution/Consequence content with the generic `"main, 0
  uncommitted"` git-facts fallback. Not data loss (`status-input.json`
  itself was never touched by this hook), but a real, silent display
  regression. Replaced the turn-boundary check with the only question
  that matters -- does `status-input.json` have real content -- and
  always re-renders it directly when it does, only falling through to
  synthetic git facts when the input is genuinely empty. Dropped the
  now-dead `.pellicle-turn-start` plumbing in both `status-reminder.sh`
  (the writer) and `status-fallback.sh` (the only reader, now gone) along
  with it.
- `joinFactors` joined up to `maxFactorsPerList` pushing/constraining items
  onto one line, then truncated the WHOLE joined string to a single
  20-word budget — measured against this project's own real
  `status-input.json` (2026-08-22), real items run a 17-word median and
  28-word max, so a longer first factor routinely consumed the entire
  budget and silently destroyed every item after it, not just shortened
  them (a genuinely load-bearing second constraining factor — "still
  self-reported by the same process... doesn't add an independent check"
  — got cut to three words and vanished). Replaced with `joinTruncated`,
  which truncates each item individually before joining, so a kept item
  is always complete or cleanly "…"-cut on its own. Also dropped
  `maxFactorsPerList` 3 → 1: at real measured lengths, one factor barely
  fits a glance line on its own, so a second/third is now dropped whole
  rather than mangled — worth revisiting (multiple factors, each its own
  line) if one per direction turns out too thin in practice. Joined with
  `; ` now, not `, ` — several real items already contain internal
  commas of their own ("schema, render, tests"), which made a
  comma-joined line ambiguous about where one item ended and the next
  began. Applied the same per-item-then-join fix to Drivers/Constraints.
- A Consequence's `next` text got the full character budget even when a
  tool-tally suffix (`— 27 calls, 547m`) would be appended right after
  it, so a technical `next` field (long identifiers eat characters faster
  than a word count alone accounts for) plus its tally could together run
  well past every other line's own length guarantee — observed live at
  162 visible characters against the file-wide 140-char cap. A word-count
  reservation was tried first and didn't help: the real case that
  triggered this sat exactly at the reserved word cap (16 words, cap 16),
  so the word-based cut never fired at all while the line still ran long
  in characters. Fixed properly with a character-based reservation
  (`truncateWordsCharCap`, `tallyReserveChars`) instead.
- The closest-call callout fired unconditionally whenever more than one
  Contribution was shown, even when every one was lopsided (9:1 next to
  8:2) — surfaced by the legibility-research lit review above, not
  observed as a live complaint: Dragan, Lee & Srinivasa's distinction
  predicts exactly this failure, a marker that fires on schedule rather
  than when it has something to reveal trains the reader to stop
  trusting it. Also fixed a related latent bug the same code touched:
  "closest" was computed as lowest for-ratio, not nearest a 50:50 split
  — those coincide whenever for ≥ against (true of every real
  Contribution recorded so far, which is presumably why this never
  showed up live), but a 1:9 split has the same lowest-ratio value as a
  legitimately close 4:6 split would have the opposite way, and the old
  code would have picked whichever one happened to sort lowest rather
  than whichever was actually closest to even. Now gated on distance
  from 0.5 (`closestCallMaxDistance`, 0.2) and omitted entirely when
  nothing shown clears it. Two new tests cover both fixes.
- Contributions' for/against bar now self-adjusts to its own column instead
  of a value that could overflow.
- The strip now auto-sizes to `content_file`'s real line count (capped) and
  only repaints when the file's mtime has actually advanced, instead of
  repainting on a blind timer regardless of whether anything changed.
- The original "skip row 1" fix for strip corruption was never the actual
  cause — corruption recurred on row 2, traced to Claude Code owning and
  redrawing the whole strip region (confirmed live: text selection alone
  triggers its own repaint). The code comment now says so; there is no
  fixed row offset that avoids this race.
- `status-reminder.sh`'s tmux pane shrink-back was skipped for any
  `pellicle-claude` session without a `status-input.json` — it exited
  before ever reaching the resize block, while `status-fallback.sh`'s
  matching expand-on-Stop has no such gate. A non-adopted project's pane
  would expand once and never shrink back.
- The expand-on-Stop resize was gated on `status.txt`'s mtime having
  advanced since the last expand. Most turns don't call `render-status` at
  all, so most Stops correctly saw no change and correctly skipped the
  resize — which meant the summary pane stayed collapsed after exactly the
  turns it was supposed to expand for. Verified live (2026-08-21) by
  shrinking the pane and re-running the hook with no `status.txt` change:
  it stayed collapsed under the old logic, expanded under the fix. Dropped
  the gate — every Stop now resizes to current content height
  unconditionally, and `.pellicle-shown-mtime` (which only existed to
  track the gate) is gone.
- The tmux resize target was capped at 15 rows, truncating real reports —
  render-status's own Context/Contributions caps still let a full report
  land past 30 lines. Dropped the ceiling; tmux's own graceful clamping on
  an oversized `resize-pane` request is the actual backstop. Also closed
  Consequences' own missing cap while here — it had no limit at all, same
  shape as the Contributions overflow fixed above, and was part
  of why a real report reached 34 lines. Same silent-keep-newest behavior
  as Context and Contributions (see Changed above for the cap value).
- The divider-row rewrite above surfaced a real byte-vs-rune bug: `len()`
  counts UTF-8 bytes, and `"─"` is 3 bytes per rune, so computing the
  divider's fill length with `len()` fell 4 visible columns short of the
  configured width. Same bug existed already, more subtly, in the
  Contributions bar's column alignment — `truncateWords` appends `"…"`
  (also 3 bytes), so `tradeoffCol`/`pad`, both computed with `len()`,
  silently misaligned the bar whenever a truncated and an untruncated
  Tradeoff landed in the same render. Typical output masked it because
  most Tradeoffs truncate together, and the overcount partly cancels.
  Both fixed with `utf8.RuneCountInString`; new regression test pairs a
  short (untruncated) and a long (truncated) Tradeoff and asserts their
  bars start at the same column.
- Two Claude Code sessions sharing one project directory could clobber each
  other's `status.txt`: `.pellicle-turn-start` and the turn-boundary check
  were shared across every session in the directory, not scoped to one, so
  session B's `UserPromptSubmit` could bump the timestamp past session A's
  already-written content and make A's next `Stop` hook mistake it for "I
  didn't write anything this turn," overwriting A's real content with the
  generic git-fallback. `status-fallback.sh` and `status-reminder.sh` now
  read `session_id` off the hook's own stdin JSON and scope the turn-start
  file to it (`.pellicle-turn-start.<session_id>`), falling back to the old
  unscoped filename when `session_id` is missing. Verified live: two
  simulated sessions with different ids no longer clobber; same-session
  self-protection still holds.
- `make install` alone left the four `status-*.sh` hooks — registered
  straight from the repo checkout, per this README — pointed at a
  repo-local `render-status` binary that `install` never built (`make
  build` was a separate, undocumented-as-required step). A fresh clone
  followed by `make install` alone left every hook failing with "No such
  file or directory," silently, behind its own `2>/dev/null || true`.
  `install` now depends on `build`. CI gained an install-smoke-test step
  that reproduces this exact scenario (clean binaries, `make install`, fire
  a hook for real) so a regression fails the build instead of shipping
  silently again.
- A Contribution's `for`/`against` accepted negative values (any pair
  summing to a positive total passed the existing `for+against == 0`
  check), which drove the bar-fill math negative and overflowed the
  rendered bar past its column. Now rejected explicitly.
- `truncateWords`' word-boundary cap never fired on a single whitespace-free
  run — a long URL, or CJK prose, which doesn't use inter-word spaces the
  way Latin scripts do — so a field like that defeated the cap entirely and
  broke the bar/column alignment math regardless of how many "words"
  `strings.Fields` counted it as. Added `maxLineChars` as a hard character
  backstop under the existing word cap.
- `status-tool-count.sh`'s write was a plain `>` redirect, which truncates
  in place — two racing invocations (a real possibility given this hook's
  own documented global, every-tool-call registration) could interleave
  into truncated, invalid JSON, not just the already-accepted lost-update
  tradeoff its own comment describes. `render-status`'s zero-value fallback
  and its own self-heal on the next real render already covered the
  corrupted-file case, but corruption is a worse failure than the tradeoff
  as documented. Now writes via `mktemp` + `mv` (a single rename syscall,
  no meaningful latency added on a hook this latency-sensitive); the same
  `writeToolCount` write path in `render-status` and its final `status.txt`
  write now go through an equivalent atomic-write-then-rename helper.

### Security

- `status-input.json`'s free-text fields (`context`, `decision`,
  `tradeoff`, `next`, `leads_to`, chain steps, `crux`/`resolved_crux`) flowed
  into `status.txt` with no control-character stripping, and both display
  paths write it to a real terminal unfiltered — `probe_redraw`'s raw
  `/dev/tty` write in `main.go`, and `pellicle-pane-render.sh`'s plain
  `cat`. Since this file is Claude-authored per turn and can echo text read
  from elsewhere in a conversation (a file, a fetched page), an embedded
  ESC byte was a real terminal-escape-injection path — OSC 52 clipboard
  writes, hidden OSC 8 links, cursor manipulation — reachable with no
  crafted tool call at all. `render-status` now strips C0 control
  characters and DEL from every free-text field immediately after decoding
  `status-input.json`, before any of it reaches `status.txt`.
- `probe_redraw`'s `content_file` had no path validation (already a named,
  accepted tradeoff — "speed over hardening") and, worse than that tradeoff
  described, painted the file's raw bytes including any embedded escape
  sequences straight to `/dev/tty` — a read-and-execute-its-escape-
  sequences primitive against any file the process could open, not merely
  a passive read. `content_file` must now resolve inside the current
  working directory, and only SGR color/style sequences (what
  `render-status`'s own output uses) survive the write to the terminal —
  every other escape sequence (cursor movement, OSC) is stripped.
- `pellicle-claude` built the tmux pane's `watch` command by hand-wrapping
  `$proj` (`$(pwd)`) in single quotes with no escaping — a project
  directory name containing a single quote broke out of the quoting into
  arbitrary shell executed inside the new pane. Now quoted with `printf
  %q`, the same pattern already used for forwarded CLI args four lines
  above.
