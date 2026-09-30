# Grasshopper UX audit, report 2: memory regression fixes

Audited: `4fea645` (2.7.0 prep), with changes on `codex/ux-audit-report2`. Synthetic records and mocked HTTP responses in the existing dependency-free Node harness. No live service or authoritative store was changed. This report covers the memory items in report 2; it does not claim browser, deployment, or real-model verification.

Reproduction: `node --test internal/gomcp/visualizer/app.test.cjs`. Eight initial regression checks failed on `4fea645`; nine additional behavior checks and two Go restore tests failed before the second implementation. All 58 Node checks now pass without skips. `go test ./internal/gomcp`, `go vet ./internal/gomcp`, `git diff --check`, and the Laws of Software changed-code heuristic gate pass. Acceptance examples are recorded in `docs/ux-audit-followup-scenarios.md` in the integration worktree; the owner's replacement AGENTS instructions accept the supplied audit outcomes without a separate approval gate.

## High

### R2-1. Live polling recovers after failures

- Seen: the baseline scheduled the next poll only on a successful fetch. The new regression check reproduced the missing retry.
- Impact: a transient failure left a Live page stale indefinitely.
- Fix: only the current request schedules its next poll in `finally`. Failures wait 3, 10, then 30 seconds; success resets the delay. Last results remain visible, with an explicit automatic-retry message.
- Check: fail three requests, observe each delay and one timer, then fire the timer and return a good response. Status returns to Live without Refresh. Existing superseded-request checks still pass.

### R2-2. Unsaved draft dismissal

- Seen: the edit form lost its text on Close, Escape, Cancel, or another memory. New checks reproduced the unguarded dismissal paths.
- Impact: a stray dismissal could erase a long correction.
- Fix: compare the current title, kind, and text with the loaded edit snapshot. A dirty dismissal offers Keep editing / Discard inside the dialog. Only Discard executes the requested action. Tab, filter, URL navigation, and explicit sign-out use the same guard. Unchanged forms close directly; expired sessions still clear private displayed state. Revision navigation is hidden while editing.
- Check: seven dismissal paths retain the draft, Keep editing preserves it, and Discard completes the intended action. Additional checks cover title changes during hash navigation, kind changes during sign-out, clean cancellation, and cancellation after a save conflict.

## Medium

### R2-3. Save limits and reasons

- Seen: the baseline used character length for titles and discarded plain-text 400 reasons.
- Impact: multibyte input failed with a generic error despite appearing within the visible title limit.
- Fix: title and content show UTF-8 byte counters beside their fields. Save is disabled beyond 512 title bytes or 32,768 content bytes. Submission enforces the same limits. The response body is read once, preserving either JSON conflict data or plain validation text. Content-size, metadata-size, and purpose errors have distinct plain sentences.
- Check: 16,385 accented characters count as 32,770 bytes and cannot submit; 257 accented title characters count as 514 bytes and cannot submit; the 512-byte boundary enables Save. Three plain-text 400 reason checks pass. Existing retry and conflict checks pass.

### R2-4. URL state and memory links

- Seen: tab, filters, search, and selected memory were absent from the URL.
- Impact: reload discarded the current view and there was no link to a memory.
- Fix: the hash holds a bounded view, project, device, platform, query, memory ID, and optional connection-request ID. Browser history follows explicit selections; hash navigation restores them. Unknown parameters are dropped, including token parameters. Memory links wait through sign-in and survive sign-out. Existing `#connect=` links can coexist with view state.
- Check: selected review scope survives a simulated reload; hash navigation restores search and Archived; a signed-out memory link opens after sign-in; approval IDs remain in combined links; unknown token parameters are removed. A dirty draft guards hash navigation. Real browser Back/Forward awaits integrated browser verification.

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

- Seen: earlier revisions could be read but had no owner restore action. Two Go tests reproduced the unsupported update payload.
- Impact: owners had to copy historical text into a new edit manually.
- Fix: Restore this revision appears on earlier revisions of active records. It posts the historical revision ID through the existing authenticated update route with the latest expected revision and a stable request ID. The existing writer restores title, text, tags, type, and purpose from that snapshot, with immutable scope/key and new confirmed owner provenance. No client-supplied scope or mixed edit fields are accepted. The existing shared save path embeds restored text before committing.
- Check: Node checks enforce payload, same-request retries, successful receipt, and conflict reload. Go checks enforce historical metadata, exact project/device/platform/key, confirmation, preserved history, startup retrieval, immediate wording and synthetic semantic search, identical replay, conflicting payload/stale expected revision, malformed fields, owner/session/origin restrictions, and whole-save refusal when embedding fails. These tests use synthetic embeddings, not the real model.

## Verified OK this pass

All 58 Node checks executed without skips, including existing owner save retry, conflict, archive, session expiration, startup preview, approval-link, exact-scope, and device polling contracts. Go gomcp tests and vet pass. The LOS changed-code gate reports no heuristic violations. No dependencies or framework were added.

Tradeoff: drafts remain local to the open page and are not autosaved. URL state uses explicit hash fields rather than adding a router. Restore reuses the transactional writer rather than copying snapshot fields in browser code. Hyrum's Law guided compatibility with approval links and existing owner updates; Gall's Law guided these local extensions; Murphy's Law guided replay, conflict, and model-failure checks.

Interaction lenses: Doherty Threshold informed the explicit pending/error/recovery feedback; Working Memory informed keeping byte constraints beside the input and distinguishing literal match cues from meaning search.

## Not tested

Browser rendering at 375×812 or desktop, light/dark screenshots, physical devices, real screen readers, Windows/Linux browsers, live Mac service, model inference, cross-harness startup, public deployment, and native browser history/draft interactions beyond the deterministic harness checks. This work does not block or complete the 2.7.0 release.
