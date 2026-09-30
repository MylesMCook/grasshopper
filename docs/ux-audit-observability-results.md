# Observability audit results

Audited: Go MCP/owner HTTP routing, owner session and memory actions, device
pairing/revocation and structured operational event output. Codex on Mac mini;
isolated `codex/ux-audit-owner-data` worktree, synthetic fixture copies.
[Acceptance examples](ux-audit-observability-scenarios.md) name observable outcomes.

Repro: The existing HTTP service had no injected structured operational logger;
owner actions and device approval lifecycle outcomes produced no shared event
trail. No private production traffic was inspected.

## 1. R5-2: Record lifecycle outcomes with a bounded privacy contract

Seen: Authentication failure, browser session ending, device revocation and owner
revision outcomes lacked consistent machine-readable events.
Impact: Operational diagnosis could not distinguish actions or correlate retries
without inspecting private application state.
Fix: Optional `Backend.Logger` accepts a standard `log/slog` logger. Typed event
fields record startup, auth_failed, pairing_started/approved/denied/expired,
device_revoked, owner_session_created/ended, owner_write and server_error.
Owner writes identify action, numeric memory ID and returned revision. Client
request IDs are one-way SHA-256 correlations under `request_id` with
`request_id_hashed:true`; arbitrary caller text never enters the log. No memory
content, title, provenance text, token, cookie, pairing ID/code/token hash, device
name, raw error, URL query, configured hostname or credential path is accepted.
Route names come from an allowlist, unknown paths become `other`; peer comes only
from parsed RemoteAddr IP. Forwarded-IP headers are ignored.
Check: Captured JSON output enforces required event fields and rejects synthetic
secret/content substrings across owner session, confirmation, pairing approval,
denial, expiry and device revocation. A synthetic 503 identifies route/status but
omits its error body. Pairing expiry records only its transition, not repeated
polls. Successful reads and expected denied-pairing polling emit no events.

## 2. R5-2: Bound repeated failure output and tracking state

Seen: Logging every failed request would permit poll spam and unnecessary volume.
Impact: Repeated errors could bury useful lifecycle evidence or grow state.
Fix: auth_failed and server_error emit at most one entry per event/allowlisted
route/peer per minute. Repetitions increment a counter; the next window's failure
includes `suppressed`. Tracking holds at most 1,024 ordinary keys plus one overflow
bucket. New keys prune expired windows; capacity overflow shares its bounded
bucket. No sleep, body buffering, forwarded-IP trust or extra dependency exists.
Check: One hundred failures produce one entry; the following minute's entry
reports 99 suppressed failures. 1,100 distinct synthetic peers keep tracking at
or below 1,025 buckets. Expected denied pairing polls stay quiet. Existing MCP
streaming tests pass with the status wrapper; ResponseController and flushing
are forwarded to the underlying response writer.

Production integration: Primary task supplies
`slog.New(slog.NewJSONHandler(os.Stderr,nil))` through Backend.Logger in the server
entrypoint, sharing hosting-worker edits. Nil Logger preserves existing quiet
behavior. Events are emitted when handler setup/actions complete; startup does
not claim a successful bind, reboot persistence, health check or deployment.

Verified: Focused `go test ./internal/gomcp -run Audit -count=1`, full Go tests,
vet and race tests for gomemory/gomcp/goclient passed. Existing tests still cover
streamable MCP requests and the five-tool contract.

Not tested: Deployed stderr capture/rotation, live failure traffic, machine
restart, centralized log forwarding and sustained adversarial load. Suppression
counts are emitted on a subsequent matching failure, not by a background timer;
quiet expired buckets need no log. No push, PR, deployment or service change.
