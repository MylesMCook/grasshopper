# What has been verified

This is observed behavior, not a claim that every app, machine, and recovery path has been exercised. Tests used synthetic records unless stated otherwise. Git history retains older release evidence.

## Backend and packages

- `go test ./...`, `go vet ./...`, and the selected race tests pass on Mac mini. [CI for 2.2.1 source](https://github.com/MylesMCook/grasshopper/actions/runs/36165978676) passed tests, vet, and native builds on macOS, Windows, and Linux.
- Extracted 2.2.1 client and server archives passed every internal SHA-256 entry on Mac mini, Beelink Ubuntu, and Work HP Windows. Each native server started from an empty task-local state, rejected anonymous `/healthz`, answered an authenticated client check, and made a verified local SQLite snapshot. Scratch listeners were stopped. This does not test persistent startup or live-data migration.
- Backend regression tests cover scoped context and search, project identity across clone paths, device/OS separation, confirmed preferences, corrections with revision conflicts, idempotent retries, full historical reads, archive/restore, immediate semantic recall, and unauthenticated rejection. These tests do not establish first-turn agent behavior.

## Actual agents

| Machine | Client | Observed result |
|---|---|---|
| Mac mini | Codex CLI 0.156.1 | Native Git marketplace install and hook trust; fresh first-turn context before tools; correction/readback; bounded outage; removal. Changed-version upgrade from 2.2.0 to 2.2.1 passed in an isolated profile. |
| Mac mini | Cursor Agent CLI 2026.09.23 | Native Git plugin, fresh first-turn context, MCP correction and historical readback, bounded outage, removal. `marketplace update` stayed on its old Git snapshot; remove/re-add fetched 2.2.1 and `setup --update` authenticated. |
| Mac mini | Claude Code CLI 2.1.280 | A published client package delivered confirmed global context in a fresh turn. Isolated plugin install, update, and removal passed. |
| Beelink Ubuntu x64 | Codex CLI 0.156.1 | Native Git marketplace install, update, and removal passed in an isolated profile. A separate fresh turn received synthetic context before tools through its trusted hook. |
| Beelink Ubuntu x64 | Cursor Agent CLI 2026.09.23 | Native Git plugin in an isolated workspace. A fresh turn found startup context absent, called `context` once, and read synthetic global record 1 plus root AGENTS.md guidance. |
| Work HP Windows x64 | Codex CLI 0.155.1 | Native Git plugin, hook trust, fresh first-turn context before tools, historical readback, bounded outage, and removal passed in scratch. |
| Work HP Windows x64 | Cursor Agent CLI 2026.09.23 | Native Git plugin and **user-level** MCP entry. Fresh turns called `context`, corrected a synthetic record through MCP `store`, read both revisions through `get`, and reported an unavailable backend without a false save. A combined plugin/MCP turn read revision 3. No GUI or real work files changed. |

The tested Windows Cursor CLI listed project-level MCP tools but did not expose them to Agent sessions. Its [documented user-level workaround](https://forum.cursor.com/t/cursor-cli-can-see-my-mcp-servers-but-agent-sessions-still-cant-use-them/171696/11) is Grasshopper's default with `--cursor-cli`. Cursor's [Git marketplace update workaround](https://forum.cursor.com/t/cli-added-git-marketplace-never-reindexes-plugin-marketplace-update-reports-0-plugins-indexed-and-cache-stays-pinned-to-the-first-commit/169587/7) is in [SETUP.md](../SETUP.md). Codex hook trust and Cursor MCP approval remain native security steps.

## Shared service and recovery

- On the private Mac mini service, fresh Codex, Cursor, and Claude Code CLI sessions received confirmed global record 8 before tools. Codex created a synthetic project record; Cursor corrected it; Codex read both revisions and archived it. A same-Git-project handoff was read from different clone paths on Mac mini and Beelink. Synthetic records were archived afterward.
- The Mac mini's encrypted off-host backup was restored to a separate path. SQLite integrity, source bytes, a synthetic record, and its prior revision matched. The backup job ran once under launchd. This is a restore rehearsal, not proof of recovery after loss of the Mac.
- The private viewer's signed browser session survived a refresh, and the user confirmed Codex Annotate could select text on the live private and public pages. The public site kept MCP and private-view routes closed.

**Still open:** Codex compaction/subagent refresh, Codex Desktop and Cursor IDE with the latest marketplace build, reboot persistence, recovery-note access from another device, and full Mac-loss recovery. The user elected CLI-first testing for Cursor and Claude; CLI passes are not Desktop/IDE passes. No legacy database was migrated. Do not label those gates complete from unit tests or older app pilots.
