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
Make setup and everyday use straightforward across machines and agent harnesses;
keep the memory model flexible as the product evolves.

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
node --test internal/gomcp/visualizer/app.test.cjs # Device polling regressions; no npm dependencies
node --test web/*.test.cjs # Public address and setup regressions; no npm dependencies
go run ./cmd/grasshopper-go-backup --help
go run ./cmd/grasshopper-go-migrate --help
go run ./cmd/grasshopper-go-bundle --help
```

The real Granite ONNX tests need `GRASSHOPPER_EMBED_TEST_ROOT` (the directory
containing model.onnx, model.onnx_data, and tokenizer.json) and
`GRASSHOPPER_ONNX_RUNTIME_LIBRARY`. Restore the pinned test assets with
`node scripts/fetch-embedding-test-assets.mjs`; CI sets both variables.
Do not call skipped model tests a real-model pass. All three asset digests
are pinned in `internal/goembed/granite.go`.

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
  `docs/verification.md`; keep `tasks.md` short and current. Do not claim
  cross-harness or deployed success from unit tests alone.

The old Linux service names in historical documentation do not prove a running
deployment. Inspect the target host, supervisor, database, ports, credentials,
and existing private routes before proposing a cutover.

## Code comments

Follow Go's doc-comment conventions for packages and exported symbols, and
describe their actual contracts. Explain non-obvious project concepts,
invariants, security boundaries, transactions, concurrency, failure, and
recovery beside the relevant code. Review affected comments when behavior
changes and remove stale or speculative claims. Do not add line-by-line
narration, comment quotas, vague TODOs, secrets, or private infrastructure
details.

Keep personal infrastructure addresses, supervisor names, credential locations
and recovery references out of public documentation as well as code comments.
Use generic self-hosting examples; keep machine-specific operator notes private.
Do not name the owner's machines, hostnames or employer devices in public files;
say "the Windows PC" or "a Linux machine" instead.

## Release cadence

Batch memory-view and setup polish into one release at most weekly. Deploy
changes that touch only the public site without a version release. Run the full
package, recovery and rollback rehearsal only when server, storage, migration or
client-bridge code changes.

## Pull request delivery

Before merging, wait for successful CI of the final pushed head and one completed,
explicitly requested independent review of the ready changes. Request the review
for a named PR and revision, then address findings and resolve conversations.
Passing CI or a quota error is not a completed review. A late review cannot protect
an already merged PR.

## PR review usage

- Be frugal with external reviewers. Run relevant local checks, inspect the
  diff, and resolve known issues before requesting a review of a ready PR.
- Prefer one external reviewer per ready PR. Copilot, Gemini and Cursor
  Bugbot are the current options when connected and available; leave Claude
  reviews unused until the owner requests them.
- When external reviewers are unavailable or quota-blocked, an explicitly
  requested independent Codex subagent review may satisfy the repository review
  requirement. The reviewer must not be an author of the changes. Record the
  unavailable reviewer, reviewed revision, completed verdict and addressed
  findings in the PR; do not claim an external review occurred. This applies to
  code and documentation receipts, so a verified rollout does not require an
  unreviewed follow-up merge.
- Request another review only when substantial risk, material changes since
  the last review, or a verified finding justifies it. Do not call every
  available reviewer or repeat reviews for minor edits.
- All code reviewers, including Copilot, Gemini, Cursor Bugbot, Claude and
  Codex, must run only when explicitly requested for that PR and revision.
  Keep automatic reviews on PR creation, draft readiness, pushes and schedules
  disabled. Do not subscribe a PR to ongoing reviews. Check existing review
  status and quota errors before retrying; do not repeatedly call a
  quota-blocked reviewer.
- Preserve applicable required review gates. If a required reviewer is
  unavailable, report the blocker and ask for an explicit exception rather
  than silently substituting another reviewer or bypassing the gate.
- The fallback above does not override GitHub-required reviewers, required
  approvals or other protected gates. Re-review material changes or fixes to
  verified findings; documentation-only receipt additions, base updates and
  minor edits need a final diff inspection, not an automatic new review.

## Linear tracking

Repository: https://github.com/MylesMCook/grasshopper
Team: Lab (`aed4ab18-a5db-4a5a-964e-4be027d7e529`).
Project: [Grasshopper](https://linear.app/mcook/project/grasshopper-9d092487eee7)
(`bff19887-4f6d-4256-956f-80d2521be8fe`). The owner authorizes finding,
creating, and maintaining outcome issues and concise progress comments for
meaningful work in this repository. Reuse an existing issue when applicable.
