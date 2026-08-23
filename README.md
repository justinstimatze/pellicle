# pellicle

> ### pellicle — updated 07:58:18
>
> - ☑ session resumed after /compact, picking up the paused live-verification thread
> - ☑ 7c7f0fc already pushed and CI-green from before the pause
>
> **Contributions**
> - ○ built render-status locally and wrote this file by hand to exercise the real Stop hook path
>   - ↑ confirms the systemMessage mechanism end-to-end, not just in unit tests
>   - ↓ binary and this file are both gitignored scratch state, not committed
>
> **Consequences**
> - ☐ read the systemMessage that lands in this transcript and confirm it matches status.txt  — 0 calls, 0s
>   ↳ closes the live-verification thread from before the compaction pause

A running status report for [Claude Code](https://docs.claude.com/en/docs/claude-code)
sessions, written by Claude itself and dropped into the transcript
automatically at the end of every turn — no request needed.

The point is staying on the same page as Claude, the way any two people
working together do: what's still true, why the last call was made, what
happens next. Enough of Claude's own recent reasoning, kept current as it
works, to catch at a glance what reading only the diff never would — a
tradeoff quietly gone unbalanced, something forgotten, a consequential
action about to happen. A running account of decisions and tradeoffs,
where a git-status widget would only show you the diff.

Three sections carry that account: **Context** (settled facts, newest
last), **Contributions** (a decision, and what pushed toward or bounded
it), and **Consequences** (the next action and where it leads) — the full
shape is in [Content model](#content-model) below.

That three-way split isn't invented in a vacuum. Human-autonomy teaming
research names the same structure — the Situation Awareness-based Agent
Transparency model's three levels (goals/actions, reasoning, projected
outcomes: Chen, Procci, Boyce, Wright, Garcia & Barnes, ARL-TR-6905, 2014;
Gao, Xu, Shen & Gao, [arXiv:2308.16785](https://arxiv.org/abs/2308.16785),
2023) map onto Context, Contributions, and Consequences independently. And
what's actually lost when that picture goes stale has a name too: a
*shared mental model* (Cannon-Bowers, Salas & Converse, 1993) — team
performance tracks how well it holds, not how tidy the project looks. In
pellicle that picture is a checkbox and an empty box, rewritten by Claude
at the end of every turn.

`status-fallback.sh` — a Stop hook every project registers once (see
[Wiring it up](#wiring-it-up)) — re-renders `status.txt` from whatever
Claude wrote to `status-input.json` this turn, then emits the hook's own
`systemMessage` field with that same text, which Claude Code shows once in
the transcript per turn. Nothing to install or launch beyond the hook
itself, and nothing painting over a shared terminal region to race
against — it's print output, delivered once, at the moment a turn ends.

## Content model

Claude writes `status-input.json` during a session — a small JSON document
with sections told apart by shape rather than printed headers:

```json
{
  "drivers": ["standing, salient facts pushing toward the project as a whole"],
  "constraints": ["standing, salient limits on it -- a deadline, a license block"],
  "context": ["settled facts, newest last"],
  "chains": [{"label": "optional, 1-4 words", "steps": ["at least 3 steps"]}],
  "contributions": [
    {"decision": "...", "pushing": ["what's driving toward this"], "constraining": ["what's bounding or limiting it"], "durability": "provisional | durable"}
  ],
  "consequences": [{"next": "the actual next action", "leads_to": "where it leads", "risky": false}]
}
```

`durability` is `"provisional"` (could still change) or `"durable"` (settled).
`render-status` turns that into `status.txt` — a markdown report that
renders as real markdown once it lands in Claude Code's own transcript.
Run `render-status -init` in a new project to scaffold a starter
`status-input.json` with each section's shape filled in as placeholders.

Every field renders behind a glyph, not a text label:

| Glyph | Meaning |
|---|---|
| `↑` / `↓` | a driving / restraining factor — Drivers and Constraints (standing, project-level), or a Contribution's own `pushing`/`constraining` (Lewin's force-field framing, reused at both scopes) |
| `☑` | a settled Context fact |
| `⛓` `┌─` `├─` `└─` | a Chain's label, and its connected steps |
| `○` / `●` | a Contribution's durability — provisional / durable, the same hollow/filled logic as `☐`/`☑` |
| `☐` / `⚠` | a Consequence's `next` action — routine / risky |
| `↳` | where a Consequence's `next` action leads |

No repeated text labels (`pushing:`, `next:`, and so on) — the glyph and
its fixed position in the report carry the meaning once you've seen it a
turn or two.

`drivers` and `constraints` (both optional) are a different scope from
Contributions' own `pushing`/`constraining`: a *standing*, project-level
fact — "ship by EOD", "can't use this library, its license blocks us" —
not something tied to one decision. Added after Contributions' own
pushing/constraining shipped and turned out too granular for what was
actually needed here: a real constraint like a license block shouldn't
live and die with whichever Contribution happened to first mention it and
then age out under `maxContributions` — it's still true and still binding
long after that decision scrolls off. Both are sparse by design (capped
at 4, keep-newest — "the salient ones," not everything) and render first,
before Context, since a reader should have the binding facts in mind
before reading anything they shaped.

Content growth is capped, silently, keeping the newest entries: 4
Drivers, 4 Constraints, 6 Context lines, 3 Contributions, 2 Consequences,
1 chain (chains need 3+ steps or `render()` errors — validate, don't
silently demote to prose). Every free-text field is also capped in
length: 20 words per field, 12 words per item for Drivers/Constraints'
multi-item lines (each item truncated individually before joining with
`; ` — not `, `, since a real item can contain its own internal commas),
and only the first entry of a `pushing`/`constraining` array renders at
all — a second or third factor is dropped whole rather than truncated
into nonsense, since a real factor already runs close to a line's own
budget on its own. A Consequence's `next` text gets a smaller character
budget when a tool-call tally will be appended right after it, so the
combined line still lands in the same length range as every other line.

`render-status <file>` also physically prunes `status-input.json` itself
down to the same keep-newest set after every render, so the input file
and the rendered `status.txt` can't drift apart on what counts as
current. Only fires with a real file path; a `stdin` render
(`status-fallback.sh`'s synthetic git-fallback JSON) has no sibling file
to prune. Safe to lose: `status-input.json` is gitignored, ephemeral
working state, not the durable record — that's git history and
`CHANGELOG.md`.

That cap is still count-only, though: a stale Consequence with nothing
new written to bump it off sits at full weight no matter how many turns
pass. `expireStaleConsequence` drops the newest Consequence once
`.pellicle-tool-count.json`'s own count — the same number shown next to a
`next` line — crosses `expireAfterCalls`, 25, a first guess rather than a
measured calibration. `render-status`'s own
divider line doubles as a small self-diagnostic strip too: how many
fields the current render is about to silently `…`-truncate ("N
field(s) over budget"), silent when clean — computed from the same
`statusdata.ExtractFields`/`OverBudget` `lint-content` checks at write
time, shared rather than duplicated a second time.

`pushing` and `constraining` (`array[string]`, required, at least one item
each) use Lewin's force-field framing — driving forces vs. restraining
forces — not a self-scored confidence number: naming a specific factor is
harder to fake than an invented ratio, and a reader can actually check
later whether a named constraint held up. An earlier design tried a
self-scored `for`/`against` ratio plus a separate `crux` field (see
[double crux](https://www.lesswrong.com/posts/exa5kmvopeRyfJgCy/double-crux-a-strategy-for-mutual-understanding))
and dropped both for not discriminating in practice, in favor of stating
the same specificity as plain prose inside `pushing`/`constraining`
instead — full story in `CHANGELOG.md`.

A Contribution's closest engineering relative is an
[Architecture Decision Record](https://adr.github.io/) (Nygard, 2011) —
deliberately diverging: an ADR is permanent, checked in, and reviewed in
a PR; a Contribution is ephemeral, capped at 3, keep-newest, and
self-authored per turn, a working sketch rather than a filed record. When
one gets silently dropped by the cap, that's a cost accepted on purpose,
not a gap — git and `status-history.jsonl` are the actual audit trail;
the strip's job is "what's live right now," not "what was ever decided."

Consequences carry `risky` (bool) — flags a `next` action as high-stakes
or hard to reverse. Renders with `⚠` instead of `☐`, so the glyph alone
carries the signal — no color to lean on in a markdown report, matching
`statusGlyph`'s own durability-shape-not-color precedent.

Consequences also carry an optional `risky_command` (string) — a literal
substring (not a regex, not free prose) of the one specific shell command a
risky `next` maps to, e.g. `"push --force"` or `"DROP TABLE"`. Meaningful
only alongside `risky: true`, and even then optional: most risky actions
aren't reducible to one matchable command and stay purely decorative, same
as `risky` on its own always has been. When it is set, `status-risk-gate.sh`
(see [Wiring it up](#wiring-it-up)) makes the flag operationally real —
escalating a matching Bash call to a confirmation prompt carrying Claude's
own stated reasoning — instead of only decorating `status.txt`. `render()`
never displays `risky_command`; it's operational metadata for that hook, not
reader-facing content. **This hook's mechanism only does anything under
Claude Code's interactive permission-approval flow** (a live `ask` prompt a
human sees before the command runs) — a workflow that never sees
permission prompts gets nothing from this hook either way, worth
checking before relying on it.

The newest Consequence's `next` line also carries a tool-call tally —
`  — 14 calls, 23m`, no threshold, no styling. It exists because
self-report can't catch a rabbit hole (disproportionate effort with no
one noticing) the way Contributions' pushing/constraining split catches
a misjudged tradeoff — the same compromised judgment that causes one
also compromises noticing it, so this had to be an externally-computed
fact. `status-tool-count.sh` (see [Wiring it up](#wiring-it-up))
increments a sidecar on every tool call; `render()` resets the count
whenever the sidecar's stored goal no longer exact-matches the newest
Consequence's `next` text — rephrasing a goal even slightly starts the
count over, a deliberate simplicity tradeoff.

A `provisional trend` sparkline appears once at least 6 renders have been
recorded to `status-history.jsonl` (gitignored, capped at 40 points) — the
fraction of recent Contributions marked `"provisional"`. Trending toward 0
(everything durable) is a confidence signal — it means Claude has stopped
hedging, independent of whether the underlying decisions were actually
right.

A second `tool-call velocity` sparkline tracks the same tool-tally count
next to the newest Consequence, over time instead of just at the
current instant — raw calls, not a percentage. It's gated and capped
independently of the provisional trend, and only shows up alongside a
currently-rendered Consequences section, the same way the provisional
trend only shows up alongside Contributions — a trend about a section
that isn't currently on screen just reads as orphaned.

The `updated HH:MM:SS` timestamp (plus the over-budget diagnostic, when
there is one) sits in the report's own header line. Markdown has no
fixed-column divider to size the way the old ANSI report did, so there's
no `-width` flag to speak of.

## Checking content before it renders

`render-status` truncates a too-long field silently — a reasonable safety
valve for a genuine outlier, but it doesn't tell the author (Claude,
writing `status-input.json`) that a field ran over budget until someone
reads the rendered report and notices the "…". `lint-content` checks the
same budget at write time instead — natively, counting words the exact
same way `render-status`'s own `truncateWords` does (plain
`strings.Fields`), so a field that passes `lint-content` is genuinely
guaranteed not to get truncated, not just probably fine.

`lint-content` doesn't depend on anything external to check this — an
earlier version shelled out to [cope](https://github.com/justinstimatze/cope),
dropped after its word counter turned out to disagree with
`render-status`'s own (full story in `CHANGELOG.md`).

```
make install                        # builds lint-content
lint-content -path status-input.json
```

Exits 1 and lists every over-budget field (word count, budget, and the
field's own text, so the fix is obvious) if any field would get silently
cut; exits 0 and prints `clean` otherwise. `internal/statusdata` holds the
schema and the two budget constants once, so `render-status` and
`lint-content` read the exact same numbers and can't drift apart on either
the shape or the count.

Wired into a real hook: `status-content-gate.sh` runs as a `PostToolUse`
hook on `Edit|Write`, globally registered but scoped to near-zero cost
everywhere else — it checks the edited path against this project's own
`status-input.json` before doing anything else, so an edit in an unrelated
project (or an unrelated file in this one) exits after a single string
comparison. When the edited file is `status-input.json` and a field comes
back over budget, it injects a capped, labelled summary of the newest
violations (`consequences[12].next, consequences[12].leads_to (+N older)`)
back to Claude as `additionalContext` — a nudge, never a block, matching
`lint-content`'s own field_terse rule being warn-only. The label shows
the *newest* violations (`tail`, not `head`), since this project's
arrays are append-only and the newest entry is the one an edit just
pushed over budget.

## Install

```
make install
```

Installs `render-status` and `lint-content` (Go binaries) to `$GOBIN`, or
`$(go env GOPATH)/bin` if unset. The five `status-*.sh` hooks (see
[Wiring it up](#wiring-it-up)) are registered straight from this repo
checkout, never copied anywhere — each one self-locates `render-status`
relative to its own script path.

`make build` builds local `./render-status` and `./lint-content` binaries
instead, version-stamped from the current git tag (`make version` prints
what a build would stamp). `make test` runs `render-status`'s test suite —
the pure functions (sparkline math, history round-tripping, render caps
and validation) are covered; the bash scripts are not.

## Wiring it up

**These hooks**, merged into `~/.claude/settings.json` once so they run in
every project but only act where a project has actually opted in (replace
`/path/to/` with wherever this repo is checked out):

```json
{
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "/path/to/status-fallback.sh 2>/dev/null || true"}]}],
    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "/path/to/status-reminder.sh 2>/dev/null || true"}]}],
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "/path/to/status-risk-gate.sh 2>/dev/null || true"}]}],
    "PostToolUse": [
      {"matcher": "", "hooks": [{"type": "command", "command": "/path/to/status-tool-count.sh 2>/dev/null || true"}]},
      {"matcher": "Edit|Write", "hooks": [{"type": "command", "command": "/path/to/status-content-gate.sh 2>/dev/null || true"}]}
    ]
  }
}
```

- `status-reminder.sh` (UserPromptSubmit) — nudges Claude to regenerate
  `status.txt` when it's gone `STALE_MINUTES` (default 7) without a real
  update, so that doesn't rely on memory alone.
- `status-fallback.sh` (Stop) — if `status-input.json` exists and has
  real content (any Context, Contribution, or Consequence), always
  re-renders `status.txt` from it — a safe no-op when it's already
  current, and a recovery on a turn where Claude wrote real content but
  never called `render-status` itself. Only falls back to synthetic git
  facts — branch, uncommitted count, and the last commit's subject +
  relative age, not a generic "nothing to report" placeholder — when
  `status-input.json` is genuinely empty. Once `status.txt` is current,
  also emits the Stop hook's own `systemMessage` with that same text —
  the transcript-dump mechanism described in
  [Content model](#content-model) above — unconditionally on every Stop,
  same reasoning as the unconditional re-render: most turns don't call
  `render-status` directly, so gating on "did anything change" would miss
  exactly the turns that should show up.
- `status-risk-gate.sh` (PreToolUse, `Bash`) — makes a Consequence's
  `risky` flag operationally real: if the most recent Consequence has
  `risky: true` and a `risky_command` that's a literal substring of the
  incoming Bash command, escalates that call to a confirmation prompt
  carrying Claude's own `next`/`leads_to` as the reason
  (`permissionDecision: "ask"`, never `"deny"` — it surfaces Claude's
  self-assessment, it doesn't get to unilaterally block anything).
  Substring match, not regex, and matches only the risky command itself —
  not every Bash call once a Consequence is risky — to avoid alert
  fatigue on an unrelated `git status` along the way. Must run
  synchronously; an async hook's `permissionDecision` has no effect.
- `status-tool-count.sh` (PostToolUse, empty matcher) — increments
  `.pellicle-tool-count.json`'s `count` on every tool call, the raw
  material behind the tool-call tally above. The empty matcher means it
  fires on *every* tool call in *every* project on the machine, not just
  ones that opted into pellicle — so it's the strictest of the five on
  speed and defensiveness: one `jq` call, no network, silent no-op the
  instant `status-input.json` is missing. Non-atomic by design; an
  occasional lost increment under a race costs nothing, since the count
  only needs to be roughly right for a human eyeballing proportionality.
- `status-content-gate.sh` (PostToolUse, `Edit|Write`) — runs
  `lint-content` against `status-input.json` after every edit to that
  specific file and injects a capped, labelled summary of the newest
  over-budget fields back to Claude as `additionalContext` (see
  [Checking content before it renders](#checking-content-before-it-renders)
  above). A nudge, never a block. Gates on the edited path matching this
  project's own `status-input.json` before running anything else, so an
  edit anywhere else — in this project or any other, since the matcher is
  global — exits after one string comparison.

All five are no-ops (not errors) in any project that hasn't opted in.
Opting a project in is `render-status -init` inside it, once — see
[Content model](#content-model) above for what that scaffolds.
