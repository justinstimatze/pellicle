#!/usr/bin/env bash
# status-tool-count.sh -- PostToolUse hook, EMPTY matcher: fires on every
# single tool call, not scoped to Bash or any other tool the way the other
# four hooks here are. Increments .pellicle-tool-count.json's `count` by
# one on every invocation -- the raw material behind the "N calls, Xm"
# tally render() appends next to the currently-active goal's own line.
# That tally exists for a specific gap: a rabbit hole (disproportionate
# effort on the current goal, unnoticed) doesn't have a mechanism the way
# a misjudged tradeoff already does (Contributions' pushing/constraining
# split), and self-report doesn't work for it -- the same compromised
# judgment that causes a rabbit hole also compromises noticing it in the
# moment. So this hook's only job is counting; render-status
# (cmd/render-status/main.go) is the one place that decides whether a goal
# is "new" and resets the counter to zero -- see syncToolCount there.
#
# Because the matcher is empty, this fires globally: in every project on
# this machine, not just ones that have adopted pellicle, and across every
# concurrent Claude Code session, not just this one -- confirmed live
# during this feature's own verification pass, where an unrelated
# concurrent session in a completely different project showed up logging
# under this exact same global registration. That makes speed and
# defensiveness the actual design constraint here, stricter than any of
# status-reminder.sh, status-fallback.sh, or status-risk-gate.sh: no
# network calls, no expensive parsing, a single jq call and nothing else,
# and ANY failure -- missing jq, missing status-input.json, an unreadable
# or malformed count file -- has to fall through to a silent, instant,
# zero-output exit 0. A bug here has the largest blast radius of anything
# in this project: everyone on this machine pays for it, on every tool
# call, not just pellicle-adopted sessions.
#
# Deliberately does NOT use `set -e`: an unguarded command failing under
# errexit still exits nonzero, and the actual requirement here is a
# *guaranteed* exit 0 on any failure, not merely a silent one. So every
# fallible step below is checked explicitly with its own `|| exit 0`
# instead of leaning on trap/errexit semantics to get there.
#
# Also deliberately does not read tool_input from stdin at all -- unlike
# status-risk-gate.sh, this hook doesn't need to know anything about which
# tool ran or what it did; it counts uniformly. Not reading stdin isn't
# just simpler, it's one less thing that can go wrong on the hot path
# every other tool call in every other session takes.
set -uo pipefail

STATUS_INPUT="$(pwd)/status-input.json"
[ -f "$STATUS_INPUT" ] || exit 0
command -v jq >/dev/null 2>&1 || exit 0

COUNT_FILE="$(pwd)/.pellicle-tool-count.json"
NOW="$(date +%s)"

# Non-atomic read-modify-write -- no flock. This can still race against
# another invocation of this same script running concurrently (two tool
# calls close together, a dispatched background agent's own tool calls
# landing under the same session -- see this feature's own live-verified
# finding that those attribute here too) and an occasional increment
# really could get lost under a true race. That's an accepted, named
# tradeoff, not an oversight: this tally is a proportionality signal for a
# human to eyeball, not an exact ledger, and real locking's complexity
# (lock acquisition, retry/backoff, stale-lock cleanup) isn't justified for
# a number that only needs to be roughly right -- especially not on a hook
# this latency-sensitive.
#
# The WRITE below, though, is via mktemp+mv, not a plain `>` redirect --
# `>` truncates in place, so two racing writes of different lengths can
# interleave into truncated, invalid JSON, not just a lost increment (a
# pressure-test panel caught this: readToolCount's zero-value fallback and
# render-status's own self-heal on the next real render already cover that
# case, but corrupting the file is a worse outcome than the accepted
# lost-increment tradeoff describes, and mv is a single rename syscall --
# no meaningful latency cost over the plain redirect it replaces).
#
# A single jq call does the parse, default, and increment together: `//`
# supplies goal/since defaults for a missing or null field, and letting jq
# itself fail on genuinely malformed JSON (or a non-numeric count) is what
# routes that case into the `|| exit 0` below instead of needing separate
# bash-side validation. A malformed file is left untouched, not repaired,
# by this script -- render-status's own goal-change check treats a
# malformed file the same as a missing one and repairs it the next time a
# real render sees a "new" goal (see readToolCount's comment in
# cmd/render-status/main.go).
if [ -f "$COUNT_FILE" ]; then
  OUT="$(jq -c --argjson now "$NOW" \
    '{goal: (.goal // ""), count: ((.count // 0) + 1), since: (.since // $now)}' \
    "$COUNT_FILE" 2>/dev/null)" || exit 0
else
  OUT="$(jq -n -c --argjson now "$NOW" '{goal: "", count: 1, since: $now}')" || exit 0
fi

TMP_FILE="$(mktemp "${COUNT_FILE}.XXXXXX" 2>/dev/null)" || exit 0
printf '%s\n' "$OUT" > "$TMP_FILE" 2>/dev/null || { rm -f "$TMP_FILE"; exit 0; }
mv -f "$TMP_FILE" "$COUNT_FILE" 2>/dev/null || { rm -f "$TMP_FILE"; exit 0; }
exit 0
