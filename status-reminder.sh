#!/usr/bin/env bash
# status-reminder.sh -- UserPromptSubmit hook. Nudges Claude to refresh
# status.txt via render-status when it's gone stale, so "update each turn"
# is enforced from the input side, not left to memory alone. Pairs with
# status-fallback.sh, which backstops the output side on Stop. Registered
# globally: runs in every project, but the nudge itself only fires in ones
# that have opted in (a status-input.json present) -- silent no-op
# everywhere else.
set -euo pipefail

PELLICLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATUS_FILE="$(pwd)/status.txt"
STATUS_INPUT="$(pwd)/status-input.json"
STALE_MINUTES="${STALE_MINUTES:-7}"

[ -f "$STATUS_INPUT" ] || exit 0

if [ -f "$STATUS_FILE" ]; then
  age_seconds=$(( $(date +%s) - $(stat -c %Y "$STATUS_FILE") ))
  if [ "$age_seconds" -lt $(( STALE_MINUTES * 60 )) ]; then
    exit 0
  fi
fi

MSG="pellicle: status.txt hasn't been refreshed in ${STALE_MINUTES}+ min. If this turn does anything worth reflecting (a decision, a fix, a milestone), regenerate it before finishing: $PELLICLE_DIR/render-status -out status.txt status-input.json with real content."
printf '{"hookSpecificOutput": {"hookEventName": "UserPromptSubmit", "additionalContext": %s}}\n' "$(printf '%s' "$MSG" | jq -R .)"
