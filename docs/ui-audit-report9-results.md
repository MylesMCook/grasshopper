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

Released patch: [2.8.3](https://github.com/MylesMCook/grasshopper/releases/tag/v2.8.3),
code `335b8e3`, marketplace `96106e5`. The owner explicitly approved completed independent Sol
review and passing final-head CI as the scoped exception for PRs #23/#24.
Copilot was quota-blocked and Gemini did not acknowledge its single request;
no external technical review is claimed. Required gates remain for other PRs.
Release and deployment are complete through the existing verified archive,
backup, recovery and rollout process. [Delivery evidence and limits](memory-acceptance.md#283-report-9-release-and-rollout-september-30).

Physical devices, real screen readers, Safari/Firefox and native agent turns
are outside this synthetic browser pass. Report 8's documentation-only PR #23
merged as `109669c` under the same owner-approved exception.

## Final UI batch after 2.8.3

Accepted scope: [LAB-235](https://linear.app/mcook/issue/LAB-235), F1–F11
from the owner's final audit. Separate Sol workers implemented the site and
memory-view slices; Codex owns integration, independent review and delivery.
No dependencies, framework, API, authentication or data changes.

- F1–F5: below 480px, commands take the full column and Copy sits below;
  native scrolling keeps a conditional fade with no scrollbar. Memory tabs
  use the same cue without an instruction line. Scope phrases stay together,
  long identities truncate with the full value available, the masthead has
  16px top spacing and the first tab aligns with the column edge.
- F6–F10: Settings uses a section-size summary and smaller task headings;
  redundant device headings are removed while accessible groups remain.
  Startup, export and sign-in wording is shorter. Setup says “Set up
  Grasshopper.” Paragraphs use pretty wrapping; headings use balanced wrapping.
- F11: both light stylesheets and DESIGN share the darker border token;
  native tests enforce at least 3:1 contrast against background and surface.
- Process: one explicitly requested, completed independent review precedes
  merge. External reviewers are preferred; an independent non-author Codex
  review can satisfy the repository requirement when they are unavailable.
  Required GitHub approvals and gates still apply. A quota failure is not review.

Integrated local checks pass: 121 Node tests, full Go tests, vet and targeted
race tests. New scope, responsive Copy and contrast regressions failed their
baseline before passing the fixes. Worker Chromium checks cover 320–1024px,
light/dark, 200% text, native keyboard scrolling, exact clipboard commands
across all three OS choices and long project identities without page overflow.
A long-project layout defect found during browser verification was corrected.
Independent non-author Sol review found no defects at `fb2f171`. Combined
browser verification then found a stale tab fade after text-only enlargement;
a narrow layout observer fixes it. A failing-before regression passes, and
explicit targeted review of `385f43d` found no defects or observer feedback loop.
Copilot remains quota-blocked; no external review is claimed.
Released [2.8.4](https://github.com/MylesMCook/grasshopper/releases/tag/v2.8.4)
from PR #26/code `765f677`, marketplace `35bf501`. Final PR and merged native
CI pass all three platforms; recovery, service, installed Mac connectors and
public-site deployment are complete. [Delivery evidence and limits](memory-acceptance.md#284-final-ui-release-and-rollout-september-30).
Physical phones, screen readers and non-Chromium browsers remain untested.
