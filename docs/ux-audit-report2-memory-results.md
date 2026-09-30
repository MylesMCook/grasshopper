# Grasshopper UX audit, report 2: memory regression fixes

Audited: `4fea645` (2.7.0 prep), with changes on `codex/ux-audit-report2`. Synthetic records and mocked HTTP responses in the existing dependency-free Node harness. No live service or authoritative store was changed. This report covers the non-gated memory items only; it does not claim browser, deployment, or real-model verification.

Reproduction: `node --test internal/gomcp/visualizer/app.test.cjs`. Eight new regression checks failed on the unmodified implementation; all 44 checks pass with the fixes. `go test ./internal/gomcp` also passes. `git diff --check` passes.

## High

### R2-1. Live polling recovers after failures

- Seen: the baseline scheduled the next poll only on a successful fetch. The new regression check reproduced the missing retry.
- Impact: a transient failure left a Live page stale indefinitely.
- Fix: only the current request schedules its next poll in `finally`. Failures wait 3, 10, then 30 seconds; success resets the delay. Last results remain visible, with an explicit automatic-retry message.
- Check: fail three requests, observe each delay and one timer, then fire the timer and return a good response. Status returns to Live without Refresh. Existing superseded-request checks still pass.

### R2-2. Unsaved draft dismissal

Pending owner approval of the proposed Gherkin. No implementation changed. The required paths include Close, Escape, backdrop, Cancel, and opening another memory; Keep editing must retain the text.

## Medium

### R2-3. Save limits and reasons

- Seen: the baseline used character length for titles and discarded plain-text 400 reasons.
- Impact: multibyte input failed with a generic error despite appearing within the visible title limit.
- Fix: title and content show UTF-8 byte counters beside their fields. Save is disabled beyond 512 title bytes or 32,768 content bytes. Submission enforces the same limits. The response body is read once, preserving either JSON conflict data or plain validation text. Content-size, metadata-size, and purpose errors have distinct plain sentences.
- Check: 16,385 accented characters count as 32,770 bytes and cannot submit; 257 accented title characters count as 514 bytes and cannot submit; the 512-byte boundary enables Save. Three plain-text 400 reason checks pass. Existing retry and conflict checks pass.

### R2-4. URL state and memory links

Pending owner approval of proposed Gherkin for reload, Back/Forward, and opening a link before or after sign-in. No implementation changed. Approval links must continue to work and secrets must stay out of URLs.

### R2-5. Hidden tabs stop polling

- Seen: the baseline had no visibility handling. The new regression check reproduced ongoing work in a hidden page.
- Impact: background memory and device requests consumed data, battery, and host work.
- Fix: hiding the page clears both timers and aborts their requests. Returning refreshes memory once and refreshes devices only when their panel is open. Superseded request completions cannot restart a loop.
- Check: the hidden-page test observes zero timers and no new fetches; return starts one memory request and the open device panel's two requests. A repeated visibility event creates no additional requests.

### R2-6. Search cues and empty states

- Seen: previews had no literal match cue and suggested All projects even when already selected. Inspection confirmed wording search combines its parsed words with OR (`internal/gomemory/search.go`).
- Impact: people could not assess why wording results matched or use the suggested empty-state recovery.
- Fix: query terms in titles and previews are bold text nodes, with no HTML interpretation. The hint distinguishes wording matches (any query word) from meaning matches (related wording); bold text identifies exact terms only. Empty results say No matching memories and suggest All projects only with a project selection.
- Check: literal `<script>` text is highlighted as text; filtered and unfiltered empty suggestions are checked. Existing search scope and one-submission checks pass. Real embedding search quality was not tested.

## Low

### R2-9. Memory accessibility and timestamps

- Seen: Connection remained an empty section after sign-in; dialog opening relied on the browser's Close focus; dates included seconds without a timezone.
- Impact: extra navigation and less useful initial dialog context, plus ambiguous update times.
- Fix: hide the entire Connection section while signed in; explicitly focus the dialog heading; format dates to the minute with the local timezone.
- Check: connected/disconnected section visibility, heading focus, and absence of seconds in the date are asserted. Real screen readers were not tested. Public-page metadata belongs to the site worker.

### R2-10. Restore an earlier revision

Pending owner approval of proposed Gherkin. No implementation changed. Any future restore must create a new confirmed revision, retain history and exact stored scope, preserve embedding/idempotency, and reject a stale expected revision without overwriting newer work.

## Verified OK this pass

All 44 Node checks executed without skips, including existing owner save retry, conflict, archive, session expiration, startup preview, approval-link, exact-scope, and device polling contracts. Go gomcp tests pass. No dependencies or framework were added.

Interaction lenses: Doherty Threshold informed the explicit pending/error/recovery feedback; Working Memory informed keeping byte constraints beside the input and distinguishing literal match cues from meaning search.

## Not tested

Browser rendering at 375×812 or desktop, light/dark screenshots, physical devices, real screen readers, Windows/Linux browsers, live Mac service, model inference, cross-harness startup, public deployment, and the approval-dependent R2-2/R2-4/R2-10 behavior. This work does not block or complete the 2.7.0 release.
