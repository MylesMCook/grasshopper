# Grasshopper UX audit, report 3: server and CLI checkpoint

Base: `4fea645`. Worker: Codex, Mac mini, isolated
report3-server worktree on detached HEAD. This is a local implementation
checkpoint, not a release or deployment. No live service or credential changed.

## R3-1. Browser navigation

Implemented: anonymous GET `/` and browser GET `/mcp` with positive
`Accept: text/html` redirect to `/visualizer/` when enabled. When disabled,
they show a small server page explaining `--visualizer`, without a broken
memory-view link. Unknown visualizer assets and `/favicon.ico` return 404.
Non-HTML, SSE and non-GET MCP requests retain authentication. Known owner API
requests also retain authentication.

Acceptance: opening a server link reaches its view or honest unavailable
guidance, while agent/API requests still require credentials.

## R3-4. Human CLI help

Implemented: no arguments, `help`, `--help` and `-h` list the human commands
first, with bridge/hook marked for agents. `connect` and `setup` parse help
before searching for an archive. Command flag help exits successfully through
the CLI entry point. Cursor and Claude command groups explain their commands.
The server's `--visualizer` help now describes owner review/edit/device controls.

Acceptance: a source-built CLI outside a package can explain its commands
without asking the user to find an archive or returning a help error.

## R3-7 and R3-8. Public operations and binary names

Implemented: public operations guidance is generic. The exact previous operator
note is preserved outside Git in the task scratch directory with mode 0600;
byte equality was verified before replacement. README links to self-hosted
operations. Root AGENTS prohibits personal infrastructure details in public docs.

README distinguishes archive aliases from Go source directory/binary names,
gives explicit output build commands, and explains that source builds do not
package model/runtime assets or client archive layout.

## Checks and gaps

- Before implementation, focused navigation and CLI help tests failed on the
  observed 401/CLI errors. After implementation both pass.
- `go test ./internal/gomcp ./cmd/grasshopper ./cmd/grasshopper-go-server` passes.
- `git diff --check` passes.
- No physical/browser navigation check, full-repository check or real-model
  verification ran in this checkpoint.
- R3-9 still needs source verification of session sign-in versus device connect
  against the combined report 2 site changes. This worktree intentionally began
  from the release-preparation commit and does not contain those changes.
- The next worker should review final route boundaries, including whether
  unknown `/visualizer/api/...` paths should return 404 before authentication.
  This checkpoint preserves that API-prefix authentication boundary.

Next action: hand over to the owner-requested gpt-6.1-sol worker for review,
remaining source verification, final report and integration. No push or delivery
has happened.
