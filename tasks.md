# Grasshopper release work

Owner: Codex. Mac checkout: `/Users/mylescook/Code/MylesMCook/grasshopper`,
branch `main`. Work HP is CLI-only while the user does real work there. Beelink
is shared with a separate host-maintenance task. All current tests use
synthetic data; no live migration or deployment is approved.

## Intended behavior

One authenticated, private Go/SQLite memory service with five tools; one
stateless Go bridge for Codex, Cursor, and Claude Code; one canonical shared
`integrations/policy/AGENTS.md`. The client package must be installable,
updatable, and removable without a second writable memory store or alternate
standing-instruction file. The former Rust CLI and code search are retired.

## Current state

- Backend, migration/backup, auth, scoped recall, revision conflict, replay,
  history, and bounded outage checks passed on Mac, Windows, and Linux.
  [Detailed evidence](docs/memory-acceptance.md). Package commit `38305a0`
  is pushed to `main`; [its CI](https://github.com/MylesMCook/grasshopper/actions/runs/36042724512)
  passed on macOS, Windows, and Ubuntu. Earlier milestones: `430cb41`,
  `86114fd`, and `95ca34a`.
- Mac client package: Codex local install/update and fresh Luna startup context
  passed; Claude local install/update and fresh Haiku `--plugin-dir` startup
  context passed. Cursor Composer received startup context from `--plugin-dir`,
  but plugin-only loading did not register its MCP server.
- Work HP Windows: Claude/Haiku received startup context; Cursor/Composer
  fetched it through a project-local MCP source and read allowlist. Codex CLI
  0.155.1 and task-local 0.156.1/Luna received synthetic record 1 revision 2
  before tool use from the 0.1.8 prompt hook. Its archive passed 24 hashes;
  the packaged hook passed a start-then-prompt fallback check. An earlier
  trusted interactive 0.1.7 turn loaded the record too. With the test server
  stopped, Codex continued without claiming memory. The isolated plugin,
  listener, token, and auth link were removed. Desktop remains untested.
- Beelink Linux: the final archive verified; Codex/Luna, Claude/Haiku, and
  Cursor/Composer 2.5 Fast received synthetic startup context. Claude read
  `context` through its plugin MCP. Cursor read it through the project-local
  MCP source. All task-local listeners were stopped and tokens removed.
- Package sources and install/update/removal instructions are in
  [integrations/plugins](integrations/plugins/README.md). Codex and Claude local
  install, update, and removal paths were rehearsed in isolated configurations.
  Cursor native Git
  marketplace/IDE install and uninstall remain unverified.

## Next action

Resolve or document Cursor plugin-only MCP loading. Windows Codex resume and
second-prompt behavior need a direct CLI check beyond the Go regression tests.
Keep `.build/` and `.playwright-cli/` untouched. A live host cutover requires
separate deployment approval, verified backup recovery, and rollback rehearsal.
