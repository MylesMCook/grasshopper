# Observed behavior

Evidence for [Grasshopper 2.3.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.0). These observations do not establish behavior in untested clients.

## Backend and release

- [Main CI](https://github.com/MylesMCook/grasshopper/actions/runs/36198401107) passed tests, vet, and native Mac/Linux/Windows builds. Seven published archives passed 280 internal file hashes; all eight GitHub assets matched local SHA256 digests. Real pinned BGE model recall tests passed on Mac.
- Tests cover scoped context and reads, corrections and prior revisions, concurrent revision conflicts, idempotency, archival, authentication, owner-only pairing, denial and expiry, revocation, restart, and bounded failure. A 2.2.5 synthetic database upgraded additively to 2.3.0; the 2.2.5 binary reopened it after rollback.
- On a synthetic Mac database, a real browser approved a client after its matching code appeared. The client authenticated; Disconnect revoked it immediately. On the live private service, a disposable client connected through Tailnet and was revoked. No master token was copied to the client.

## Actual clients

| Machine | Harness | Observed |
|---|---|---|
| Mac mini, macOS | Codex CLI 0.156.1 | Installed 2.3.0 local plugin; a fresh read-only turn received the confirmed global preference before tools. |
| Mac mini, macOS | Cursor Agent CLI 2026.09.23-86fc751 | Updated 2.3.0 wiring; a fresh Ask turn received the same preference before tools. |
| Mac mini, macOS | Claude Code CLI 2.1.280 | Updated 2.3.0 plugin; a fresh print turn received the same confirmed preference before tools. An unrelated claude.ai connector still returned 502. |
| Work HP, Windows | Codex CLI 0.155.1 | 2.3.0 marketplace entry paired to the private server with a device token; authenticated `check` passed. A clean-room fresh turn received the confirmed Mac-saved preference before tools. SSH stayed open after the model turn; unrelated MCP OAuth errors appeared. |
| Work HP, Windows | Cursor Agent CLI 2026.09.23-86fc751 | 2.3.0 user MCP wiring and startup hook installed. A clean-room fresh turn saw MCP tools but no startup memory. A separate turn called `grasshopper/context` once and read the confirmed Mac-saved preference. Direct execution of the hook returned that context, so the missing first-turn injection remains a Windows CLI integration gap. |

Earlier [2.2.5 evidence](https://github.com/MylesMCook/grasshopper/blob/v2.2.5/docs/memory-acceptance.md) includes Claude Code CLI, Beelink Linux recovery, scoped readback, and CLI outage probes. Those checks were not all repeated for 2.3.0. CLI checks do not prove Codex Desktop or Cursor IDE startup behavior.

## Live service and site

- Mac mini launchd runs 2.3.0 on loopback 8106, behind the unchanged Tailnet-only HTTPS route. Anonymous `/healthz` returned 401; authenticated local and Tailnet health returned 200; the private viewer returned 200. SQLite integrity was `ok`, with 10 records and 25 revisions after upgrade. The consistent pre-upgrade snapshot restored into a separate test server with the same counts.
- The post-upgrade backup job exited successfully. Its local snapshot passed SQLite integrity and includes device-token metadata; the job uploads to and checks the encrypted off-host R2 repository. Independent Beelink recovery was last observed on 2.2.5, not repeated after this update.
- [The public site](https://usegrasshopper.com/) returned 200 for home, setup, and memory view. Setup shows 2.3.0. Public `/mcp` and `/visualizer/` returned 404. The published Mac client download returned 200.

Still unverified: reliable Work HP Cursor first-turn injection, current Codex Desktop and Cursor IDE sessions, reboot persistence, compaction/subagent refresh, and actual host failover. [Cursor documents `sessionStart` as fire-and-forget](https://cursor.com/docs/hooks); a connected MCP tool alone does not guarantee first-turn context. No legacy production database was migrated.
