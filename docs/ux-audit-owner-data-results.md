# Owner data and pagination audit results

Audited: Go owner browser API, revision provenance, browser-session persistence,
export, owner purpose selection, list pagination and conditional responses.
Codex on Mac mini; isolated `codex/ux-audit-owner-data` worktree. Synthetic
fixture copies only. No live database, deployment, dependency or framework changes.

Repro: Before implementation, `TestOwnerConfirmPreservesProvenanceAndReplay`
failed because the confirm action returned 400. `TestOwnerExportHasFullContentAndArchivedChoice`
failed because export returned 404. Existing session code validated a stateless
30-day HMAC and logout only cleared the browser cookie. Existing owner list
returned one budget-limited result with no traversal cursor.

## 1. R4-1: Preserve saved-by provenance when confirming

Seen: Owner update always supplied memory-view provenance, even for unchanged text.
Impact: Confirmation attributed the agent's original observation to the owner.
Fix: Explicit `action: "confirm"` loads the requested historical revision and uses
its full text, metadata and provenance. It accepts no edited fields. Existing edit
and restore behavior retains owner provenance; writer revision checks and request
fingerprints still enforce concurrency and identical retries.
Check: Native test confirms attribution, confirmation, revision 2 and identical
replay; existing owner edit/history/restore/conflict checks pass.

## 2. R4-2: Make browser sign-out revoke access

Seen: A copied cookie remained usable after local sign-out.
Impact: Removing one browser cookie did not invalidate its copied credential.
Fix: Seven-day HMAC cookies additionally require their SHA-256 hash in SQLite
`owner_sessions`. DELETE revokes one session; owner-only POST revoke-all removes
all browser sessions. Token rotation invalidates HMAC signatures. Paired devices
and master bearer stay independent. Only hashes and expiry times are persisted.
Check: Copied-cookie rejection, other-session preservation, revoke-all, new-handler
restart, actual database reopen, cookie attributes and master/device independence
pass. Login allows ten failed attempts per peer IP per five minutes, with no sleeps;
its map caps at 1,024 peer windows and expires them. Valid master credentials
reset the failure bucket and can sign in even after invalid attempts. Persistent browser sessions
cap at 256; expired rows are removed on sign-in, a full table fails explicitly.
Compatibility: Old stateless cookies have no persisted hash and require sign-in.
The schema addition is idempotent, preserves memory/history and is included in
existing SQLite backups; an older binary ignores the table but cannot enforce
these revocations. Do not roll back authentication expectations to an old binary.

## 3. R4-3: Export complete owner memories

Seen: No owner download route existed.
Impact: The owner could inspect individual records but could not export them.
Fix: Owner-only exact-Origin JSON export streams one SQLite read snapshot,
including every full Record field and complete content. It excludes quarantined
legacy data. Scope/view/purpose filters apply, or `all` selects all owner memories.
Check: Long content, archived inclusion, master/anonymous/paired authorization and
foreign/missing Origin tests pass. Interrupted streams remain invalid JSON rather
than closing a misleading partial document.

## 4. R4-4: Filter the owner view by memory kind

Seen: Purpose was absent from owner list/search filters.
Impact: The owner could not request only lessons, decisions or another kind.
Fix: `purpose` is a validated closed enum on BrowseScope and is applied in SQL
before lexical/vector ranking or result budgets. Owner counts also respect it.
Agent context/search scope contracts are unchanged.
Check: Filtered owner list and search contain only lessons; invalid kinds fail.

## 5. R5-3: Traverse stable pages without omissions

Seen: A fixed result budget stranded later records.
Impact: Records outside the first page were unreachable as list rows.
Fix: Pages use an ordered snapshot of ID/revision references, not mutable update
keys. Content is loaded from immutable historical revisions. Exact snapshot total,
next cursor, limit and 32-KiB record budget replace list omission references.
Check: All 300 records are visited once across concurrent updates and a new write;
old revisions stay consistent. Changed filters/limit and expired cursors fail.
Snapshots live in handler memory for ten minutes: at most 64 snapshots,
50,000 references per snapshot and 100,000 aggregate references. Older snapshots
are evicted when capacity is needed. Narrow filters for a larger collection.
A restart, expiry or eviction requires reloading the list. This transient cache
contains references only, never writable memories or duplicated content.

## 6. R5-4: Avoid repeated unchanged response bodies

Seen: Polling always transferred list/startup JSON; exclusion details were unlimited.
Impact: Unchanged responses and large startup exclusion inventories repeated work.
Fix: List and startup set deterministic ETags and return bodyless 304 for matching
If-None-Match after authentication/input validation. Tags include the memory
revision aggregate so an update to a detail beyond the bounded preview changes the
tag. Startup returns at most 100 exclusion details and an exact total; agent
Context selection and startup selected records remain unchanged.
Check: Initial 200, second 304/no body, write-changed tag and 130-record bounded
startup exclusion tests pass.

## Frontend contracts

- POST `/visualizer/api/context`: flat existing scope/view fields plus `purpose`,
  `limit` (default 50, range 1–100) and `after` (opaque string). Response retains
  Page/filter/count/server fields and adds `next` (empty at end), `total` (exact
  snapshot count). `omitted`/references are empty for paged lists. A cursor binds
  scope, view, purpose and limit; invalid/expired cursors return 400 with a reload
  message. Records are 240-rune previews; details use the record route.
- POST `/visualizer/api/search`: existing `{scope,query}`; scope gains `purpose`.
  Empty purpose means All; valid values are preference, decision, lesson, handoff,
  observation. This does not add fields to agent scope schemas.
- POST `/visualizer/api/update`: `{action:"confirm",id,expected_revision,request_id}`.
  Do not send title/content/purpose for confirm. Existing edit payload is unchanged;
  `action:"edit"` is optional. Read revision 1 to display originally-saved-by after
  an owner edit. Existing restore_revision payload remains unchanged.
- POST `/visualizer/api/export`: `{scope,all,include_archived}`. Response is an
  attachment named `grasshopper-memories.json` with `{records:[full Record...]}`.
  `all:true` clears scope/view/purpose; include_archived defaults false. include_archived adds archived records to the selected Saved or review view;
  the archived view remains archived. Read-only paired credentials are rejected.
- DELETE `/visualizer/api/session`: revokes this cookie and returns 204.
  POST `/visualizer/api/session/revoke-all`: owner cookie or master bearer plus
  exact Origin, returns 204 and clears the browser cookie. Paired tokens remain.
- List/startup POST support If-None-Match and ETag; 304 has no JSON body.
  Startup adds `not_loaded_total` alongside capped `not_loaded`.
- Go asset registration includes `theme-init.js` for the frontend worker's new
  external prepaint script; JS content is supplied by that separate slice.

Verified: `go test ./...`, `go vet ./...`,
`go test -race ./internal/gomemory ./internal/gomcp ./internal/goclient`, and focused
owner/preview tests passed. The normal Go embedding tests may skip real ONNX tests
without model assets; no real-model claim is made here.

Not tested: Browser integration of the new controls, deployed service behavior,
other harnesses/machines, GUI refresh, reboot persistence, load testing at the
snapshot/session caps and live backup/restore. Frontend and structured audit-log
integration belong to the primary task. No push, PR or deployment performed.

## Follow-up: Keep scope selection inside the export snapshot

Review found that separate scope-key enumeration and streaming selection could
combine old scope keys with newer content when another connection committed in
between. Export now resolves project/device/platform predicates and selects full
records in one SQLite SELECT, with bound parameters and safe JSON scope parsing.
No preliminary scope read remains. This preserves global/applicable-dimension,
All projects, purpose, review and archived selection behavior.

Focused native scope-boundary tests pass. A deterministic WAL test commits a new
project scope and an existing record change together while the export stream is
open: the current download contains the entire older state and the next download
contains both changes. The former pre-SELECT gap is removed structurally; this
test exercises mid-stream isolation rather than injecting a production test hook
into that removed gap. Full Go tests/vet/race are rerun for the follow-up.

The same single-statement scope predicates also select ordered page references.
Filter-choice queries may observe newer metadata, but they cannot narrow or widen
the record/revision snapshot. This closes the equivalent scope-enumeration gap in
pagination without changing cursor or agent contracts.
