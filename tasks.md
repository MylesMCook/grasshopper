# Shared agent memory release

Owner: Codex. Machine: macOS arm64. Checkout: `/Users/mylescook/Code/MylesMCook/grasshopper`, branch `main`. Synthetic work only; no live deployment or data migration.

## Intended behavior

One authenticated, private Go/SQLite service with `context`, `store`, `search`, `get`, and `archive`; one stateless Go client for Codex Desktop, Cursor, and Claude Code on macOS and Windows; one canonical `integrations/policy/AGENTS.md`. Scope, confirmed preferences, revision history, provenance, idempotency, full reads, and backup/rollback remain inspectable. The old Rust CLI and code search are retired by user decision.

## Current evidence

- Go backend, model-backed recall, migration/backup, authentication, revision conflicts, idempotency, immediate search, archive/restore, and scope isolation passed local automated checks on Mac and Work HP. Synthetic desktop pilots on both machines exchanged Go-backed records through the former Rust bridge; exact limits are in [acceptance evidence](docs/memory-acceptance.md).
- The native Go client passed transport/context/outage checks on Mac, Work HP, and Beelink. Fresh Mac Cursor GUI, Mac/Work HP Claude Code, and Mac Codex CLI sessions used it across both machine directions. Codex Desktop and Work HP Cursor GUI still need fresh Go-client checks.
- Beelink Ubuntu 24.04.4 x64 passed the full Go suite with real BGE, vet, race checks, authenticated five-tool client/server behavior, backup/reopen, and an extracted Linux archive with 52 valid checksums. Its existing older Grasshopper services on 8106/8107 remained active; the synthetic listener on 18106 was stopped and its token removed. The Linux archive is in task-local evidence. See [acceptance evidence](docs/memory-acceptance.md).
- The Go-only baseline is pushed to `main`; [Go CI](https://github.com/MylesMCook/grasshopper/actions/runs/35933965315) passed on macOS, Windows, and Linux. The Linux bundle filename fix passed Mac tests and Beelink's extracted-binary smoke check; cross-platform CI for that fix is pending.
- Root/nested AGENTS.md loading was observed in pilot desktop sessions, with the Windows Codex nested read order self-reported rather than independently traced. Interactive resume/compaction and full install/removal remain unverified.
- No live Grasshopper service was confirmed on this Mac. Do not infer one from historical documentation. No persistent route, supervisor, real profile, or live database has been touched.

## Next action

Finish fresh Codex Desktop and Work HP Cursor GUI Go-client checks, then interactive lifecycle and installation/removal checks. A live cutover remains a separately approved step after host inspection, backup verification, and a rollback rehearsal. Preserve unrelated `.build/` and `.playwright-cli/` directories.
