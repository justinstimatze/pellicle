#!/usr/bin/env bash
# status-content-gate.sh -- PostToolUse (Write|Edit) hook. Dogfoods
# lint-content: runs it against status-input.json right after Claude
# edits that specific file, and surfaces any over-budget field back to
# Claude as additionalContext before the next render() truncation ever
# happens. Same registration pattern as status-reminder.sh/
# status-fallback.sh/status-tool-count.sh: global matcher (fires on every
# Write|Edit in every project), silent no-op unless the edited file is
# THIS project's status-input.json.
#
# Warn-only, matching lint-content's own field_terse rule (a @shape
# violation, never a reject-action rule) -- this hook only ever injects
# additionalContext, never permissionDecision. Blocking the edit would
# claim a stronger authority than the underlying check has: a field
# running long is a nudge to tighten it, not a reason to refuse the write.
#
# Gate order matters for speed: check the edited path BEFORE running jq
# on the hook's own stdin, so an edit anywhere else in this project (or
# any of the many other projects this hook is globally registered in)
# exits after one string comparison, no jq/lint-content invocation at
# all -- Edit|Write is a much hotter path than status-risk-gate.sh's
# already-scoped Bash-only matcher.
set -uo pipefail

PELLICLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATUS_INPUT="$(pwd)/status-input.json"
LINT_BIN="$PELLICLE_DIR/lint-content"

[ -f "$STATUS_INPUT" ] || exit 0
[ -x "$LINT_BIN" ] || exit 0
command -v jq >/dev/null 2>&1 || exit 0

INPUT="$(cat)"
FILE_PATH="$(printf '%s' "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null || true)"
[ -n "$FILE_PATH" ] || exit 0
[ "$FILE_PATH" = "$STATUS_INPUT" ] || exit 0

# 2>&1, not 2>/dev/null: lint-content's own summary line ("N field(s)
# over...") goes to stderr, not stdout (see its main(), the os.Exit(1)
# branch) -- discarding stderr here discarded the one line this hook
# depends on, caught live on the first pipe-test against real data
# (SUMMARY came back empty against a file with 53 known violations).
OUTPUT="$("$LINT_BIN" -path "$STATUS_INPUT" 2>&1)" || true

# lint-content prints "lint-content: clean" with no per-field detail on a
# clean run, and a "lint-content: N field(s) over..." summary line
# otherwise -- grep the summary line rather than re-parsing exit code,
# same clean-marker-not-exit-code reasoning this project used while
# lint-content still shelled out to cope-gate (it now checks natively,
# but keeps the same output contract this hook depends on).
SUMMARY="$(printf '%s' "$OUTPUT" | grep -o '[0-9]\+ field(s) over.*' || true)"
[ -n "$SUMMARY" ] || exit 0

# Field labels only (e.g. "context[2]"), not lint-content's own word
# count/budget/text detail -- additionalContext lands in Claude's own
# input for the next turn, and the label plus render-status's published
# budget is enough to act on; the offending text is already right there
# in the file Claude just wrote. Capped at 5 labels with a
# "+N older" tail -- a backlog of aged-out entries (from before this
# budget existed, not new content) can otherwise grow arbitrarily large
# before capStatusData's own pruning catches up to it, and dumping all
# of them into additionalContext would be the exact density problem
# lint-content exists to prevent, just relocated into the hook instead
# of the report.
#
# tail -5, not head -5: caught live via a real dogfooding test -- head
# always showed the same 5 oldest context[] entries regardless of what
# was actually just edited, because extractFields walks in file order and
# the backlog of pre-existing violations outnumbers whatever one edit
# adds. The nudge fired every time but never once surfaced the field that
# was actually just written. This project's arrays are append-only by
# convention (a new entry is always added to a tail, never inserted
# mid-array), so the newest violations -- the ones actually actionable
# right now -- sit at the end of ALL_LABELS, not the start.
#
# cut -d: -f1, not a trailing-`:` strip via sd: sd's default match is
# over the WHOLE input, not per line, so a `:$` anchor only ever matches
# the very last line -- caught live on this exact command, where every
# line but the last kept its colon. cut on a fixed delimiter has no such
# whole-input surprise. Same reasoning rules out `paste -sd ', '` for the
# join: paste cycles through a multi-character delimiter one character
# per line instead of using it as one separator, so ", " alternates
# comma and space rather than joining with both every time -- also
# caught live. `paste -sd, -` then a global `,`→`, ` swap avoids both
# traps at once.
ALL_LABELS="$(printf '%s' "$OUTPUT" | grep -o '^[][a-zA-Z0-9_.]\+:' | cut -d: -f1)"
TOTAL="$(printf '%s\n' "$ALL_LABELS" | grep -c .)"
SHOWN="$(printf '%s\n' "$ALL_LABELS" | tail -5 | paste -sd, - | sd ',' ', ')"
if [ "$TOTAL" -gt 5 ]; then
  LABELS="$SHOWN (+$((TOTAL - 5)) older)"
else
  LABELS="$SHOWN"
fi

MSG="lint-content: $SUMMARY in status-input.json ($LABELS) -- render-status will silently truncate these with … at maxLineWords=20/maxStandingFactorWords=12. Tighten them now if they're still load-bearing, or leave them if the truncation is fine."
printf '{"hookSpecificOutput": {"hookEventName": "PostToolUse", "additionalContext": %s}}\n' "$(printf '%s' "$MSG" | jq -R .)"
