# Shared agent memory release

Owner: Codex. Machine: macOS arm64. Checkout: `/Users/mylescook/Code/MylesMCook/grasshopper`, branch `main`. Synthetic work only; no live deployment or data migration.

## Intended behavior

One authenticated, private Go/SQLite service with `context`, `store`, `search`, `get`, and `archive`; one stateless Go client for Codex Desktop, Cursor, and Claude Code on macOS and Windows; one canonical `integrations/policy/AGENTS.md`. Scope, confirmed preferences, revision history, provenance, idempotency, full reads, and backup/rollback remain inspectable. The old Rust CLI and code search are retired by user decision.

## Current evidence

- Go backend, model-backed recall, migration/backup, authentication, revision conflicts, idempotency, immediate search, archive/restore, and scope isolation passed local automated checks on Mac and Work HP. Synthetic desktop pilots on both machines exchanged Go-backed records through the former Rust bridge; exact limits are in [acceptance evidence](docs/memory-acceptance.md).
- The native Go client passed transport/context/outage checks on Mac, Work HP, and Beelink. Fresh Mac Cursor GUI, Mac/Work HP Claude Code, and Mac Codex CLI sessions used it across both machine directions. App-specific checks remain unverified and are deferred by user choice; use CLIs for agent testing.
- Beelink Ubuntu 24.04.4 x64 passed the full Go suite with real BGE, vet, race checks, authenticated five-tool client/server behavior, backup/reopen, and an extracted Linux archive with 52 valid checksums. Its existing older Grasshopper services on 8106/8107 remained active; the synthetic listener on 18106 was stopped and its token removed. The Linux archive is in task-local evidence. See [acceptance evidence](docs/memory-acceptance.md).
- The Linux bundle fix is pushed to `main` as `430cb41`. [CI for that commit](https://github.com/MylesMCook/grasshopper/actions/runs/35948655567) passed tests, vet, and builds on macOS, Windows, and Linux; race checks passed on macOS and Linux. The extracted Linux bundle also passed on Beelink.
- Root/nested AGENTS.md loading was observed in pilot desktop sessions, with the Windows Codex nested read order self-reported rather than independently traced. Interactive resume/compaction and full install/removal remain unverified.
- [State-evolution research check](docs/research-state-evolution.md): synthetic bank/retrieval checks and fresh Codex, Cursor, and Claude Code CLI answers passed on macOS. Cursor's project-local CLI example needed a required empty `deny` array and inclusion in the portable bundle. The corrected exact example passed headless reads and fresh readback; headless write was rejected, while interactive one-time approval acknowledged a synthetic correction. Work HP CLI and longer correction chains remain open.
- No live Grasshopper service was confirmed on this Mac. Do not infer one from historical documentation. No persistent route, supervisor, real profile, or live database has been touched.

## Next action

Work HP access is paused at the user's request; do nothing there until they explicitly resume it. On Mac, continue the longer correction-chain and CLI lifecycle checks. App-specific testing is deferred to the user if a problem appears. A live cutover remains a separately approved step after host inspection, backup verification, and a rollback rehearsal. Preserve unrelated `.build/` and `.playwright-cli/` directories.
