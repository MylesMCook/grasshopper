# Grasshopper repository instructions

Before substantive work, read `integrations/policy/AGENTS.md`. That file is
the one canonical shared memory-use policy; this file governs repository work.
If startup has not supplied memory context, call the connected Grasshopper
`context` tool once when available. Read nested AGENTS.md before editing files
in its scope.

## Product boundary

Grasshopper is one authenticated, self-hosted SQLite memory service. Codex
Desktop, Cursor, and Claude Code use one stateless client. Clients do not
maintain writable memory databases. The remote service exposes only `context`,
`store`, `search`, `get`, and `archive`; it cannot index server files.

Remember explicit preferences, accepted decisions, verified lessons, and short
handoffs. Scope global/project/device/platform records before retrieval or
writes. A missing Git remote or durable project ID is unresolved, never global
project context. Keep confirmed preferences stable until an explicit correction
with expected revision. Preserve record IDs, provenance, revision history,
legacy quarantine, and idempotency. New writes must be searchable immediately.

Local code search from the old Rust CLI is retired for this release.
Do not restore a second language runtime, separate writable store, transcript
mining service, or parallel instruction files.

## Commands

```sh
go test ./...
go vet ./...
go test -race ./internal/gomemory ./internal/gomcp ./internal/goclient
go build ./cmd/grasshopper             # Stateless client bridge and hooks
go build ./cmd/grasshopper-go-server   # Fresh or converted database
go run ./cmd/grasshopper-go-backup --help
go run ./cmd/grasshopper-go-migrate --help
go run ./cmd/grasshopper-go-bundle --help
```

The real BGE ONNX tests need `GRASSHOPPER_BGE_TEST_ROOT` and, when outside the
Mac research layout, `GRASSHOPPER_ONNX_RUNTIME_LIBRARY`. Do not call skipped
model tests a real-model pass. The model and tokenizer digests are pinned in
`internal/goembed/bge.go`.

## Layout and safety

- `internal/gomemory/` owns scoped SQLite reads, writes, history, search,
  migration, and backup. `internal/goembed/` owns local ONNX embeddings.
- `internal/gomcp/` exposes the authenticated five-tool HTTP service.
  `internal/goclient/` supplies the one stateless harness bridge and AGENTS.md
  hook adapter. `cmd/` contains their binaries and the archive builder.
- Keep shared behavioral policy only in `integrations/policy/AGENTS.md`; use
  root or genuinely narrower nested AGENTS.md for project instructions.
- Use synthetic or shadow database copies for tests. The migration never
  mutates its source. Back up and rehearse recovery before any live cutover.
- Keep credentials out of Git, output, logs, exports, and repository identity.
  Bind the server to explicit loopback; private routing requires approval.
- Record actual harness, version, OS, and observed behavior in
  `docs/memory-acceptance.md`; keep `tasks.md` short and current. Do not claim
  cross-harness or deployed success from unit tests alone.

The old Linux service names in historical documentation do not prove a running
deployment. Inspect the target host, supervisor, database, ports, credentials,
and existing private routes before proposing a cutover.
