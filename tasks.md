# Shared agent memory release

Owner: Codex. Machine: macOS arm64. Checkout: `/Users/mylescook/Code/MylesMCook/grasshopper`, branch `main`. Synthetic work only; no live deployment or data migration.

## Intended behavior

One authenticated, private Go/SQLite service with `context`, `store`, `search`, `get`, and `archive`; one stateless Go client for Codex Desktop, Cursor, and Claude Code on macOS and Windows; one canonical `integrations/policy/AGENTS.md`. Scope, confirmed preferences, revision history, provenance, idempotency, full reads, and backup/rollback remain inspectable. The old Rust CLI and code search are retired by user decision.

## Current evidence

- Go backend, model-backed recall, migration/backup, authentication, revision conflicts, idempotency, immediate search, archive/restore, and scope isolation passed local automated checks on Mac and Work HP. Synthetic desktop pilots on both machines exchanged Go-backed records through the former Rust bridge; exact limits are in [acceptance evidence](docs/memory-acceptance.md).
- A native Go client bridge and hook now pass compiled transport/context/outage checks on Mac and Work HP. Go-only portable archives on both OSes passed checksums and extracted-binary smoke tests. Fresh Mac Cursor GUI and Mac/Work HP Claude Code and Codex CLI sessions now read or corrected synthetic records through the Go client, including both machine directions over temporary forwards. Codex Desktop and Work HP Cursor GUI still need fresh Go-client checks.
- Root/nested AGENTS.md loading was observed in pilot desktop sessions, with the Windows Codex nested read order self-reported rather than independently traced. Interactive resume/compaction and full install/removal remain unverified.
- No live Grasshopper service was confirmed on this Mac. Do not infer one from historical documentation. No persistent route, supervisor, real profile, or live database has been touched.

## Next action

Finish Codex Desktop and Work HP Cursor GUI Go-client checks, then CI and archive checks on final source. Commit/push only the intended repository changes. A live cutover remains a separately approved step after host inspection, backup verification, and a rollback rehearsal. Preserve unrelated `.build/` and `.playwright-cli/` directories.
