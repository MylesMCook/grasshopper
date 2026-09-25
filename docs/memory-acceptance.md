# Observed behavior

This records tests actually run for [2.2.5](https://github.com/MylesMCook/grasshopper/releases/tag/v2.2.5). Git history retains older pilots.

## Backend and packages

- [CI 36189022975](https://github.com/MylesMCook/grasshopper/actions/runs/36189022975) passed tests, vet, and native builds on macOS, Linux, and Windows, plus race checks on Mac and Linux. All seven release archives passed 280 internal checksums; GitHub asset hashes and the published `SHA256SUMS` matched local files.
- A fresh extracted Mac server loaded the pinned ONNX model, returned 401 without a token and 200 with one, served current record 10 revision 6 plus revision 5 under full project/device/OS scope, and reported semantic search ready. The new scoped `get` regression failed before the fix. Tests also reject other-project, other-platform, and legacy reads.
- Installer tests cover exact Cursor and Claude read permissions, preservation of other settings, idempotence, malformed config, and rollback. No write permission is preapproved. Backend tests cover scoped recall, corrections, concurrency conflicts, idempotency, history, archive/restore, and authentication.

## Actual clients

| Machine | Harness | Observed result |
|---|---|---|
| Mac mini, macOS | Codex CLI 0.156.1 | 2.2.5 plugin; fresh turn received record 10 revision 6 before tools; full scoped `get` read current and prior revision 5. |
| Mac mini, macOS | Cursor Agent CLI 2026.09.23 | 2.2.5 client; same first-turn and full-read result. A 2.2.4 headless `get` had been denied; setup now adds only the three [documented read permissions](https://prod.cursor.com/docs/cli/reference/permissions). |
| Mac mini, macOS | Claude Code CLI 2.1.280 | 2.2.5 plugin; same first-turn and full-read result. A denied headless `get` was fixed with three [supported MCP read rules](https://code.claude.com/docs/en/permissions). An unrelated claude.ai Grasshopper connector still reports 502; the local plugin works. |

All three direct 2.2.5 hook outage probes reported an unavailable backend within 0.1 seconds without claiming a save. Earlier real CLI outage and Windows/Beelink harness tests are in Git history; they were not repeated for this release. CLI behavior does not prove Codex Desktop or Cursor IDE behavior.

## Live service, recovery, and site

- Mac mini launchd runs 2.2.5 against the original SQLite database and private Tailnet route. Local health returned 401 anonymously and 200 with the token; Tailnet health and visualizer returned 200. Full scoped `get` read current and prior revisions, and semantic search was ready. A live handoff correction acknowledged record 10 revision 7, with revision 6 still inspectable.
- Beelink Ubuntu restored the latest encrypted R2 snapshot using a Bitwarden note retrieved there, checked SQLite integrity (10 records, 24 revisions), and served record 10 revision 6 from a separate 2.2.4 loopback server. The test listener, restored database, vault session, and temporary credentials were removed. The Mac then uploaded a new postrelease backup; its local snapshot passed integrity and contains revision 7 with 25 historical revisions. This is independent-host recovery evidence, not an actual failover.
- The [public site](https://usegrasshopper.com/) returned 200 for home, setup, and memory view. All three 2.2.5 server download links returned 200. Public `/mcp` and `/visualizer/` returned 404. The private viewer remained available.

Not yet observed: current Codex Desktop and Cursor IDE sessions, compaction/subagent refresh, reboot persistence, and actual host failover. Work HP was not touched for 2.2.5; no legacy database was migrated.
