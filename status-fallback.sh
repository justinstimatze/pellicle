#!/usr/bin/env bash
# status-fallback.sh — Stop hook backstop for pellicle's status.txt, and the
# source of the transcript dump: status.txt is markdown (render-status's own
# canonical format, see its render() doc comment), so once this hook has a
# current status.txt it also emits the hook's own systemMessage JSON with
# that same text, landing in Claude Code's transcript once per turn --
# race-free the same way tmux's separate pane used to be, since nothing is
# painting over a shared terminal region here either. Registered globally:
# runs in every project, but only actually touches anything when the
# project has opted into the rich content model (status-input.json
# present) -- never in an unrelated project just because the hook fires
# everywhere. Always re-renders status.txt from whatever's actually in
# status-input.json, falling back to synthetic git facts only when that
# file is genuinely empty (nothing real to show at all). Renders through
# render-status itself rather than hand-building markdown here, so the
# fallback can never visually drift from the real format.
#
# This used to gate on a turn-boundary check instead (status.txt's mtime
# vs .pellicle-turn-start, written by status-reminder.sh's matching
# UserPromptSubmit hook) -- rewrite only if Claude hadn't already written
# status.txt THIS turn. That conflated "Claude didn't call render-status
# this specific turn" with "there's nothing current to show": any turn
# that didn't personally re-render (most turns in a fast-moving
# conversation -- reading a file, answering a question, opening something
# in an editor) got its real narrative silently replaced by the generic
# git-facts fallback, discarding content status-input.json still had
# sitting right there. Caught live, the same turn it happened: real
# Contribution/Consequence content from one turn was gone from status.txt
# by the next, replaced with a bare "main, 0 uncommitted" line -- not a
# data-loss bug (status-input.json itself was never touched), but a real
# display regression, and a silent one. Always re-rendering the real
# input instead is a safe no-op when nothing changed, and actively
# recovers content on the turn Claude wrote to status-input.json but
# never got around to calling render-status itself. Same reasoning is why
# the systemMessage below fires unconditionally on every Stop too, not
# gated on status.txt having changed this turn.
set -euo pipefail

PELLICLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATUS_FILE="$(pwd)/status.txt"
STATUS_INPUT="$(pwd)/status-input.json"

[ -f "$STATUS_INPUT" ] || exit 0

# git_summary_json emits the real, always-available facts both content
# branches below fall back on: branch, uncommitted count, and the last
# commit's own subject + relative age -- concrete history, not a generic
# "nothing to report" placeholder. commit/commit_age are empty strings in
# a repo with no commits yet; the jq below only includes that line when
# commit is non-empty.
git_summary_json() {
  local branch changed now commit commit_age
  if branch=$(git -C "$(pwd)" rev-parse --abbrev-ref HEAD 2>/dev/null) && [ -n "$branch" ]; then
    :
  else
    branch="no-git"
  fi
  changed=$(git -C "$(pwd)" status --porcelain 2>/dev/null | grep -c . || true)
  now=$(date '+%H:%M:%S')
  commit=$(git -C "$(pwd)" log -1 --format='%s' 2>/dev/null || true)
  commit_age=$(git -C "$(pwd)" log -1 --format='%cr' 2>/dev/null || true)
  jq -n --arg branch "$branch" --arg changed "$changed" --arg now "$now" \
    --arg commit "$commit" --arg commit_age "$commit_age" \
    '{branch: $branch, changed: $changed, now: $now, commit: $commit, commit_age: $commit_age}'
}

# has_real_content: status-input.json has at least one Context,
# Contribution, or Consequence entry -- the only question that actually
# matters here. Whether THIS turn was the one that wrote it is irrelevant;
# real content from any prior turn is still real content.
has_real_content() {
  jq -e '((.context // []) | length) + ((.contributions // []) | length) + ((.consequences // []) | length) > 0' \
    "$STATUS_INPUT" >/dev/null 2>&1
}

# Always re-render the real input when there's anything in it -- a safe
# no-op if status.txt is already current, a recovery if it isn't (see
# this file's own header for why "current" can't be a turn-boundary
# check). Falls through to the same synthetic git facts used when
# status-input.json is genuinely empty if render-status also rejects it as
# invalid (a Contribution missing pushing/constraining, a Chain with too
# few steps) -- render-status's own validation exits 1 on that, and under
# `set -e` a bare call on its own would abort this whole script before the
# systemMessage below ever runs, silently skipping this turn's transcript
# entry instead of showing something. Testing it as an `if` condition
# keeps `set -e` from firing on that specific failure, same as
# has_real_content's own `jq -e` already relies on.
if has_real_content && "$PELLICLE_DIR/render-status" -out "$STATUS_FILE" "$STATUS_INPUT"; then
  :
else
  git_summary_json | jq \
    '{
      context: (
        ["\(.branch), \(.changed) uncommitted, \(.now)"]
        + (if .commit != "" then ["last commit: \(.commit) (\(.commit_age))"] else [] end)
      ),
      contributions: [],
      consequences: []
    }' | "$PELLICLE_DIR/render-status" -out "$STATUS_FILE"
fi

# status.txt IS the markdown report now (render()'s own canonical format --
# see its doc comment) -- reuse it verbatim as the transcript dump, rather
# than rendering a second time. `jq -Rs` slurps the whole file as one JSON
# string, matching the JSON-construction convention status-reminder.sh and
# status-content-gate.sh already use for their own hookSpecificOutput; `-c`
# keeps the output a single compact line the same way those two already are
# (jq pretty-prints by default, which would spread the object across
# several lines -- still one JSON value, but needlessly different from
# every other hook here). This must be the only thing this script ever
# prints to stdout -- hook JSON parsing expects exactly one object.
if [ -f "$STATUS_FILE" ]; then
  jq -Rsc '{systemMessage: .}' < "$STATUS_FILE"
fi
