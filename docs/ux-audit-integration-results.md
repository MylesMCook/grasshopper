# Grasshopper UX audits 1–6: integrated results

Implemented in `codex/ux-audit-integration`, based on released 2.7.0 and release
records through `2031446`. This is local implementation evidence, not a claim of
publication or changes to the private service. The primary checkout was not
edited. No dependencies or framework were added.

Workers for the remaining owner-data, hosting, frontend and review slices were
explicitly `gpt-6.1-sol`, with one editor per worktree. The primary task integrated
commits and ran combined checks. Earlier completed slices were retained. Audit
outcomes are accepted requirements under the owner's updated canonical AGENTS.md;
no additional scenario-approval round is required for these concrete fixes.

## Findings and outcomes

| Report | Implemented outcome and detailed evidence |
| --- | --- |
| 1 | Tabs-first mobile layout, collapsed filters, bounded initial list, Devices link, pinned controls, inline review, sign-in help, invalid-address status and server-first site with screenshot. [Memory report](ux-audit-report1-results.md), [site/connector report](ux-audit-site-connector-results.md). |
| 2 | Poll recovery/visibility, draft protection, byte limits/errors, URL state, search cues, setup commands, hostname checks, accessibility/time metadata, revision restore and status vocabulary. [Memory report](ux-audit-report2-memory-results.md), [site report](ux-audit-site-connector-results.md). |
| 3 | Browser entry guidance, overflow, prepaint theme, CLI help, keyboard skip link, landmarks, generic public operations notes and binary-name guidance. Browser sign-in and agent connect terminology remain distinct. [Server report](ux-audit-report3-server-results.md), [frontend report](ux-audit-frontend-followup-results.md). |
| 4 | Confirm preserves provenance; edits expose the original source; revocable seven-day sessions; full JSON export; Kind filter; archive Undo; release download table; favicon/canonical/OG metadata. [Owner-data report](ux-audit-owner-data-results.md), [frontend report](ux-audit-frontend-followup-results.md), [hosting report](ux-audit-hosting-results.md). |
| 5 | Server-bound credential reuse and separate device-token path; bounded structured logs; stable owner pagination; conditional responses and bounded preview omissions; backup verification; contextual startup errors; scoped agent hints. [Credential report](ux-audit-report5-security-results.md), [owner-data report](ux-audit-owner-data-results.md), [observability report](ux-audit-observability-results.md), [agent report](ux-audit-agent-surfaces-results.md). |
| 6 | Quickstart proxy support including omitted HTTPS default port; numeric handoff guidance; verified-download and service-template docs; offline owner-token rotation; tool-neutral policy; monospace handoffs; server archive cleanup. [Hosting report](ux-audit-hosting-results.md), [agent report](ux-audit-agent-surfaces-results.md), [frontend report](ux-audit-frontend-followup-results.md). |

## Corrections and deliberate limits

R6-2 offered either shorter startup text or a numeric handoff-writing rule. This
change takes the policy option: under 600 characters. A regression confirms
confirmed decisions are selected before a long handoff, so that handoff does not
displace them. Four of five distinct 200-byte decisions fit the synthetic
3,000-byte budget once full metadata is included; adding a 1,700-byte handoff
removes none. Unconfirmed decisions remain excluded by design. The live session
from the audit was not inspected; its omission cause is not assumed.

Session revocation adds persistent hashed session state. Existing stateless
cookies require sign-in again on upgrade. Owner-token rotation is explicitly
offline; old credentials end after restarting with the replacement token.
Paired device credentials remain valid. No credentials were rotated here.

Export is JSON, with full text and provenance, filtered by scope/kind/view or all
memories and an archived option. Markdown export and a backup restore command
were optional audit suggestions and were not added. Backup verification checks
a standalone snapshot and refuses pending WAL/journal state. Signing and
notarization infrastructure remains an owner decision; no security setting was
changed. Public release notes and site sources are ready for publication, but
published assets were not changed.

## Combined browser evidence

Chromium through the installed Playwright CLI, 375×812 and 1280×960, light/dark,
against a task-local loopback server and 103 invented records. A fixed synthetic
test token was used, never a real credential.

- Mobile tabs start at 249px; first card at 635px and its preview at 743px.
  Filters are collapsed and page width is 375px. Initial list shows eight cards.
- A 32,768-character memory scrolls inside its dialog. Close remains at 25px,
  actions end at 787px, and edit actions remain within the viewport.
- Show more reached all 103 unique IDs across server pages. Needs review handled
  a 96-character unbroken title without horizontal overflow.
- Kind survives reload. An open-memory URL reopens the same record and focuses
  the heading. Escape prompts before discarding edits; Keep editing preserves
  the typed text. Save, original-author attribution, archive/Undo and historical
  restoration completed through the real synthetic HTTP routes.
- Downloaded JSON contains 103 unique records, revision/provenance fields and
  the full maximum-size memory. Sign out everywhere returns to sign-in.
- An aborted list poll automatically recovers to Live with the correct summary
  when requests resume, without pressing Refresh.
- Locally built setup page at 375px wraps every command, rewrites Windows plugin
  commands, copies the selected command, loads the screenshot and favicon, and
  honors saved dark theme with a light system preference. Canonical and OG
  metadata exist. Early script placement is checked separately by Node tests.

The browser script was resumed after two test-harness issues: a mistaken filter
button ID and an overly transient status assertion. These were corrected in the
local evidence scripts; no product fix was inferred from either harness error.

Review found and fixed four frontend integration defects: stale failure status
on 304, deferred text-selection redraws, overlapping page snapshots and accepting
incomplete JSON exports. A browser regression also moved advanced controls below
the list after they pushed the first memory off-screen. Backend review found a
scope-enumeration gap in export's claimed snapshot, fixed with a single scoped
streaming query and concurrent-write regression.

## Verification and delivery boundary

Focused failing regressions are recorded in each slice report. Combined Go
suite, vet, native Node tests, race checks and pinned real-model checks are run
from the integration checkout; the final task record records their status.
No real screen reader, physical phone, Safari/Firefox, native Windows/Linux,
Tailscale Serve deployment, supervisor restart/reboot, or live agent turn was
performed for these changes. The verified branch is published as [PR #16](https://github.com/MylesMCook/grasshopper/pull/16).
PR #16 merged as `6321229` after macOS, Ubuntu and Windows CI passed
head `9922260`; the merged tree matches that tested head. No release or
deployment occurred.
