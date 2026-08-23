#!/usr/bin/env bash
# status-risk-gate.sh -- PreToolUse (Bash) hook that makes Consequences'
# `risky` flag operationally real instead of purely decorative. Registered
# globally: runs before every Bash call in every project, but only ever acts
# in one that has opted in (status-input.json present) AND whose most recent
# Consequence names a specific, identifiable command via risky_command --
# silent no-op everywhere else, same "not every project has opted in"
# pattern as status-reminder.sh and status-fallback.sh.
#
# This hook has a stricter obligation than either of those two: a bug here
# that errors, hangs, or over-matches runs on EVERY Bash command in EVERY
# project, not just pellicle-adopted ones. So it errs aggressively toward
# doing nothing whenever anything is uncertain -- missing status-input.json,
# unreadable, unparseable JSON, no jq -- exit 0, no output, every time.
#
# Deliberately narrow by design, not by accident: an earlier version of this
# feature considered firing on every Bash call whenever the most recent
# Consequence was risky, with no command-matching at all. That would force a
# confirmation prompt on every unrelated command run on the way to a risky
# one (a test suite, a git status) -- exactly the alert-fatigue failure mode
# this whole feature exists to avoid. Undifferentiated gating trains humans
# to rubber-stamp; differentiated gating that only fires on the actually-
# relevant command is the whole point. So the match below is a literal,
# fixed-string substring check (risky_command is arbitrary Claude-authored
# text that could contain *, ., or other characters a regex would treat as
# metacharacters) against the incoming command, nothing broader.
#
# Only ever escalates to "ask" -- never "deny". This hook is Claude's own
# self-assessment surfacing itself to the human via the permission prompt
# that would exist anyway; it doesn't get to unilaterally block a tool call,
# which would be a stronger claim than pellicle's own risk-flagging deserves
# to make unsupervised. Must run synchronously (no "async": true in its
# settings.json registration) -- an async hook's permissionDecision has no
# effect, since by the time its output arrives the tool call has already run.
set -euo pipefail

STATUS_INPUT="$(pwd)/status-input.json"

command -v jq >/dev/null 2>&1 || exit 0
[ -f "$STATUS_INPUT" ] && [ -r "$STATUS_INPUT" ] || exit 0

INPUT="$(cat)"
CMD="$(printf '%s' "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null || true)"
[ -n "$CMD" ] || exit 0

# Full consequences array, not render()'s capped-at-2 view -- this hook
# reads status-input.json directly, it doesn't go through render-status.
LAST="$(jq -c '.consequences[-1] // empty' "$STATUS_INPUT" 2>/dev/null || true)"
[ -n "$LAST" ] || exit 0

RISKY="$(printf '%s' "$LAST" | jq -r '.risky // false' 2>/dev/null || true)"
[ "$RISKY" = "true" ] || exit 0

PATTERN="$(printf '%s' "$LAST" | jq -r '.risky_command // empty' 2>/dev/null || true)"
[ -n "$PATTERN" ] || exit 0

# Fixed-string substring match, not a regex -- risky_command is arbitrary
# Claude-authored text (e.g. "push --force", "DROP TABLE") that could
# contain regex metacharacters, and a case/glob match on the literal text
# is what avoids any surprise from that.
case "$CMD" in
  *"$PATTERN"*) ;;
  *) exit 0 ;;
esac

NEXT="$(printf '%s' "$LAST" | jq -r '.next // ""')"
LEADS_TO="$(printf '%s' "$LAST" | jq -r '.leads_to // ""')"

jq -n --arg next "$NEXT" --arg leads_to "$LEADS_TO" '{
  hookSpecificOutput: {
    hookEventName: "PreToolUse",
    permissionDecision: "ask",
    permissionDecisionReason: "pellicle flagged this: \($next) -- \($leads_to)"
  }
}'
