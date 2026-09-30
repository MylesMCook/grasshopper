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

Implementation and relevant failing-before checks are in progress in isolated
worktrees. Browser verification uses synthetic data, both themes, mobile/desktop
and enlarged text. Required review, CI, packaging, recovery and deployment are
pending. No physical-device, real-screen-reader, non-Chromium or native-AI result
is claimed.
