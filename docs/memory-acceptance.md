# Observed behavior

Evidence for [Grasshopper 2.3.2](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.2), with earlier checks retained below. These observations do not establish behavior in untested clients.

## 2.3.2 client update (September 26)

- [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36246295264) passed on Mac, Linux, and Windows. Seven release archives passed their embedded SHA-256 checks. The published checksum file matched the locally verified one. The marketplace branch is `25bd3cb`.
- Mac mini: Codex marketplace plugin and Claude plugin report 2.3.2; Cursor user hooks and MCP point to the 2.3.2 client. Beelink: Codex and Claude plugins report 2.3.2; Cursor user hooks and MCP point to the 2.3.2 client. Work HP: Codex plugin reports 2.3.2 and Cursor user wiring points to the 2.3.2 client; Claude was not installed. All three packaged clients passed authenticated `check` without a write.
- Direct installed `SessionStart` hooks for Codex, Cursor, and Claude loaded policy and live context in 22–26 ms on Mac and 93–99 ms on Beelink. Work HP Codex and Cursor hooks did so in 0.59–0.62 seconds. The packaged Mac client returned the unavailable fallback in 2.03 seconds against a synthetic stalled server. These are hook timings, not fresh model-turn timings. A read-only Work HP Codex CLI probe was stopped after 90 seconds without an answer; fresh GUI sessions and Work HP Cursor CLI first-turn injection were not rechecked.
- Work HP's first `codex plugin add` reported Windows “Access is denied” while backing up an old cache entry. The installed plugin list and new cache then reported 2.3.2, and that binary authenticated and loaded context. The old 2.3.0 cache was left intact. This install warning still merits investigation on a fresh Windows update.
- The public setup page serves 2.3.2 links (Cloudflare deployment `898d00cf-095a-4625-9b30-438766fc0504`). Home, setup, and memory view returned 200; public `/mcp` and `/visualizer/` returned 404. The private Mac server remains on compatible 2.3.0 with its database and route unchanged.

## 2.3.1 package and cross-machine check (September 26)

- [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36223607139) passed on Mac, Linux, and Windows. All seven archives passed embedded SHA-256 checks. The Linux archive extracted under `/opt` as root with readable plugin manifests; its client authenticated using a Beelink-only device credential. The running Mac server stayed on compatible 2.3.0.
- Fresh Mac Codex CLI 0.156.1, Cursor Agent CLI 2026.09.23-86fc751, and Claude Code CLI 2.1.280 turns saw confirmed global record 8 before tools, including after their 2.3.1 client updates. Beelink Codex CLI 0.156.1, Cursor Agent CLI 2026.09.23-86fc751, and Claude Code CLI 2.1.282 did the same from a clone of this Git remote on 2.3.1. The two machines read one server; no client memory database was created. Beelink Claude's first attempt hit a transient OAuth refresh error; an immediate retry succeeded.
- An actual Codex `search` tool call took 92 ms on Mac and 191 ms from Beelink. The full agent turns took 14.1 and 15.2 seconds. Fresh no-tool turns took Mac Codex 6.3 and 69.8 seconds, Mac Cursor 8.3–8.6 seconds, Mac Claude 5.3–7.0 seconds, Beelink Codex 6.2–7.1 seconds, Beelink Cursor 20.2–38.2 seconds, and Beelink Claude 9.8 seconds on the successful retry. These are individual observed turns, not latency guarantees. Model and harness time varied much more than MCP time.
- Direct authenticated MCP measurements, median / p95 in milliseconds:

  | Call | Mac loopback | Beelink → Mac |
  |---|---:|---:|
  | `context` | 1.2 / 4.7 (25) | 6.5 / 11.9 (30) |
  | `get` | 0.6 / 2.1 (25) | 8.0 / 20.0 (30) |
  | semantic `search` | 94 / 691 (12) | 205 / 282 (30) |

  Counts are in parentheses. Fresh hook processes took 25–27 ms median on Mac and 74–77 ms on Beelink (15 calls per harness). In an isolated synthetic service, model load took 255 ms once, embedding took about 4.8 ms median, `store` took 5.9 ms, and `archive` took 1.1 ms. These direct series used 2.3.0 clients; 2.3.1 changed package permissions, not the call path. Live search is slower and more variable than isolated inference; its cause has not been isolated. Agent response time is excluded from these direct measurements.
- With a task-local config pointing at a closed loopback port, Mac `check` failed in 20 ms and its startup hook returned an explicit unavailable fallback in 23 ms. Beelink took 11 ms and 5 ms. These test a refused connection, not a slow or blackholed network; no write was attempted.
- Mac no-tool turns: Codex high/low took 5.0–6.3 seconds (four turns), Sol/Luna at low took 6.1–6.9 seconds (four), and plugin-on/off took 5.3–7.7 seconds (four, no consistent difference). Cursor took 7.3–7.8 seconds (two); Claude took 4.8–6.2 seconds (two). Cursor's first CLI event arrived at 4.9 seconds with partial streaming. These small read-only samples do not rank model speed or coding quality.
- In four read-only Codex code lookups, low reasoning took 14.4–17.3 seconds and high took 28.2–33.7 seconds. All four found `ProjectIdentity` in `internal/goclient/scope.go`. Shell commands completed almost instantly; the agent spent most of the turn before the first command or reasoning between commands. This supports low effort for bounded lookups, not a global setting change for harder coding work.
- The hook emits 5.3 KB for Codex/Cursor and 8.8 KB for Claude, including Claude's project AGENTS.md adapter. A stalled synthetic backend held the old hook for 5.0 seconds; the new two-second limit returned the unavailable fallback in 2.02 seconds. A task-local build still loaded live context in 23–207 ms across all three harness modes. Ordinary MCP requests keep their five-second timeout. The limit reached installed clients in 2.3.2.
- The public [setup page](https://usegrasshopper.com/setup/) now links to 2.3.1 server archives for Mac, Windows, and Linux. All three download links returned 200. Home and memory-view pages returned 200; public `/mcp` returned 404. Cloudflare deployment: `11b2f748-e484-427d-bc11-22e39e86e50c`.

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

On Work HP, the 2.3.0 client checked an unreachable loopback server using a task-local config. It failed in 28 ms and reported that no memory changed; the test config was removed.

## Live service and site

- Mac mini launchd runs 2.3.0 on loopback 8106, behind the unchanged Tailnet-only HTTPS route. Anonymous `/healthz` returned 401; authenticated local and Tailnet health returned 200; the private viewer returned 200. SQLite integrity was `ok`, with 10 records and 25 revisions after upgrade. The consistent pre-upgrade snapshot restored into a separate test server with the same counts.
- The post-upgrade backup job exited successfully. Its local snapshot passed SQLite integrity and includes device-token metadata; the job uploads to and checks the encrypted off-host R2 repository. Independent Beelink recovery was last observed on 2.2.5, not repeated after this update.
- [The public site](https://usegrasshopper.com/) returned 200 for home, setup, and memory view. Setup shows 2.3.0. Public `/mcp` and `/visualizer/` returned 404. The published Mac client download returned 200.

Still unverified: reliable Work HP Cursor first-turn injection, current Codex Desktop and Cursor IDE sessions, reboot persistence, compaction/subagent refresh, and actual host failover. [Cursor documents `sessionStart` as fire-and-forget](https://cursor.com/docs/hooks); a connected MCP tool alone does not guarantee first-turn context. No legacy production database was migrated.
