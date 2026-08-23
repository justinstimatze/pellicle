# Security Policy

## Scope

Two of this project's five hooks (`status-tool-count.sh`, and
`status-risk-gate.sh` if you choose to register it) run with an empty or
`Bash` matcher, which means they fire on *every* matching tool call in
*every* Claude Code session on the machine — not just projects that have
adopted pellicle. Each one is written to no-op immediately (missing
`status-input.json`, missing `jq`, any parse failure) in a project that
hasn't opted in, so the realistic report here is "this hook does something
in a project it shouldn't," not "this hook is slow everywhere." See each
script's own header comment for its specific blast-radius reasoning.

## Reporting

If you discover a security vulnerability, please email justin@justinstimatze.com directly rather than opening a public issue or PR.

I'll acknowledge receipt within 7 days and aim to provide an initial assessment within 30 days. We can coordinate on a disclosure timeline — defaulting to 90 days from initial report unless circumstances warrant otherwise.

Thanks for helping keep this project and its users safe.
