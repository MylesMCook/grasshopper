# UI audit report 11: scannability

Baseline: live 2.8.4, `405a5a3`. The owner's concrete report is accepted scope
for [LAB-236](https://linear.app/mcook/issue/LAB-236). Codex coordinates isolated
`gpt-6.1-sol` setup, home and memory-list workers, integration and review.
Use existing files, DESIGN tokens and native controls; no new dependencies.

## Accepted outcomes

- S1–S3: distinct display and step headings, a linked overview, and one server
  action per numbered line. Keep exact release/checksum links, local token
  guidance and the optional private-proxy path.
- S4–S7: a remembered agent selector, All initially, main command order labels,
  a native disclosure for the Claude alternative and a short Copy label at all
  widths. Changing agents or OS must preserve exact command copying and feedback.
- S8–S9: numbered connection actions with a copyable prompt, secondary recovery
  guidance and an explicit check that a saved preference returns next session.
- H1–H4: one reading measure, stronger lead, three labelled facts, actual desktop
  and phone crops of invented memory data, and a returning-user memory-view action.
- M1–M4: compact previews and 24px gaps, attention-only state labels, relative
  dates with complete timestamp titles, safe project grouping, collapsed filters
  at every width and a signed-out-only subtitle. Search stays visible.
- The public address page separates its simple action from the token privacy note.

The simplest density choice is a compact default, with full text available in
its existing dialog. No new density preference or control is needed. Group
ordinary browse results by project with stable group/member order; preserve
search ranking and startup order. Project-filtered global memories must remain
visibly global. Device/platform specificity and raw identity remain inspectable.

## Accepted examples

These concise examples describe the outcomes; project-local Node/native tests
and actual browser assertions enforce them. They are not an adopted BDD runner.

```gherkin
Scenario: Show only the chosen connector
  Given setup initially shows all supported agents
  When the owner chooses Cursor and reloads setup
  Then only the Cursor connector instructions are shown
  And the selected operating system still determines every command
  And choosing All shows every connector again

Scenario: Recover from unavailable saved preferences
  Given browser storage is blocked or contains an unknown agent value
  When setup opens
  Then all connector options are available
  And commands and copy actions still work

Scenario: Scan a project without losing global identity
  Given a project-filtered memory list includes project and global memories
  When the owner scans the compact list
  Then project labels do not repeat unnecessarily
  And global memories remain identified as global
  And every memory retains its title control and full-text dialog

Scenario: Group a paginated browse list safely
  Given the owner browses memories from several projects
  When they show another page
  Then each memory appears once under its project
  And search ranking and startup-preview order remain unchanged
  And focus stays on the existing pagination action

Scenario: Reveal filters intentionally
  Given the owner opens the memory view on a phone or laptop
  Then search is visible and filters are collapsed
  And active filters are indicated by the toggle
  When they expand filters
  Then project, device, platform and kind choices are available
```

## Verification and delivery

S1–S9, H1–H4, M1–M4 and the public address wording are implemented in
existing files. Native regressions demonstrated agent-selection, grouped-page
and signed-in/filter failures before the fixes. All 132 Node tests, Go tests,
vet and targeted race tests pass. Local model tests that lack the pinned runtime
are not counted as real-model verification; native release CI follows.

Playwright CLI with invented records checked setup at 320/375/1024px in both
themes and normal/enlarged text (12 cases), all eight copied commands across
three OS choices, remembered agent selection and keyboard overview anchors.
Home/address/404 checks cover 48 viewport/theme/text combinations. Parent checks
cover compact memory layout, view overflow, exact project/global identity and all
29 project-filtered records through pagination without duplication; focus stays
on the pagination button while more records remain. Search/startup ordering and
review attribution are enforced by the native Node suite.

On a 1024×768 synthetic project list, six card starts fit and the first is at
296px; an all-project list adds group headings and shows five. The original
unfiltered baseline first card was at 717px. Titles retain 44px targets and
complete text remains in the dialog. Phone previews use two lines. Expanded
filters and longer review titles naturally use more space.

The screenshots are direct browser crops of three invented example records:
749×662 desktop with expanded filters and 343×615 phone with collapsed filters.
Both public previews select the phone image below 600px, and the build copies
both assets. No real memories or credentials appear in either image.

The first explicitly requested independent Sol review identified two P2s: the
new command disclosure must refresh its overflow cue, and relative timestamp
text must receive hover for its full-date tooltip. Both have targeted failing-
before/native checks and fixes. A targeted final-revision review follows.

Patch 2.8.5 awaits one completed independent review and successful
final-head CI before merge, then verified packaging, recovery and rollout.
No physical-device, real-screen-reader, non-Chromium or native-AI result is
claimed. Delivery receipts will record actual release/deployment evidence.
