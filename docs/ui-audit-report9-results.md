# UI audit report 9: details, text and formatting

Baseline: 2.8.2, `df877ec`. The owner's concrete report is accepted scope for
[LAB-234](https://linear.app/mcook/issue/LAB-234). Codex coordinates separate
`gpt-6.1-sol` memory-view and public-site workers, integration and review.
Use existing files and DESIGN tokens; preserve authentication, data, export
and startup contracts. No dependencies or framework changes.

## Accepted outcomes

- D1–D10: compact, discoverable token help; no signed-out result heading;
  tightly grouped titles; aligned header/results column; clear refresh spacing;
  one mobile tab row; visible filter state/count; clean preview truncation;
  primary inline confirmation; honest startup filters and exact-match hints.
- T1–T10: consistent project/author labels with raw identity preserved;
  archived-only state label; intentional kind casing; clear export/session/device
  copy; Settings naming; nonduplicated empty search; view/kind scope summary;
  contextual browser title; plain scope guidance. T4 is verification of the
  existing visual casing, not a behavior change.
- P1–P9: single-column scrollable command strips with overflow cues; unobtrusive
  mobile copy actions; copyable Claude archive command; visible current page;
  matching keyboard/header order; OS-specific download names; consistent terms
  and address example; skip links on all public pages.

## Verification and delivery

Implemented D1–D10, T1–T10 and P1–P9 in the existing stylesheets, markup and
scripts. Kind labels deliberately use sentence-case text and CSS uppercase.
Export says “Selected filters”: search words and startup-preview limits do not
apply, so “What you're viewing now” would overstate the existing export contract.
Unknown author identifiers and raw project IDs remain available.

Independent `gpt-6.1-sol` review found four gaps in the first integration:
mobile header order, empty manual filter counts, unconditional scroll guidance
and startup-export wording. All were corrected and reviewed again. Integration
also caught width-only header alignment; main and masthead now share centered
edges. Actual title text sits 7.6px below its category with a 44px target;
Refresh has a 16px gap. The public screenshot was refreshed from invented data.

Local checks pass: 117 Node tests, `go test ./...`, `go vet ./...` and the
targeted Go race suite. Seven new memory regressions and two public regressions
were demonstrated against their earlier baseline before passing the fixes.
Local checks do not establish fresh real-model inference; native CI supplies
that separate release gate.

Playwright CLI Chromium checks pass:

- Ten memory layout/theme combinations at 320, 375, 768, 1024 and 1280px:
  aligned edges, one tab row, targets, title lifecycle, filters and URL reload,
  startup hints, inline primary action, empty search, Settings and sign-in errors.
- Sixteen Saved/review/archived/startup state combinations at 375 and 1024px,
  light/dark, plus owner-edit/original-source dialogs, retained dirty drafts and
  pending pairings in all four width/theme combinations; 320px at 200% text.
- Twenty-four public page/width/theme combinations: DESIGN tokens, seven exact
  clipboard commands, keyboard scrolling, conditional overflow cues, no overlap
  or page overflow. Worker checks separately cover all four public skip links,
  visual/keyboard header order and 200% text reflow.

Planned patch: 2.8.3. The owner explicitly approved completed independent Sol
review and passing final-head CI as the scoped exception for PRs #23/#24.
Copilot was quota-blocked and Gemini did not acknowledge its single request;
no external technical review is claimed. Required gates remain for other PRs.
Release and deployment are authorized when ready, through the existing archive,
verified-backup, recovery and rollout process.

Physical devices, real screen readers, Safari/Firefox and native agent turns
are outside this synthetic browser pass. Report 8's documentation-only PR #23
merged as `109669c` under the same owner-approved exception.
