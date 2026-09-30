# Grasshopper UX audit, report 7: acceptance and results

Scope: the owner-supplied post-2.8.0 report, based on `faa7118`. Implementation
and release target 2.8.1. Codex coordinates isolated `gpt-6.1-sol` workers.
The owner authorized release and deployment once verification establishes
readiness. No new dependencies or framework are required.

## Accepted behavior

```gherkin
Scenario: Start a server for the selected operating system
  Given the owner selects macOS, Windows or Linux on setup
  When they read or copy the server instructions
  Then the archive and quickstart command match that operating system
  And sign-in and optional private HTTPS instructions are available
  And the same choice applies to connector commands

Scenario: Find first sign-in instructions
  Given the owner starts a quickstart server with a chosen data directory
  Then startup names the actual token file and explains how to use it
  And the memory view explains the default locations and custom-directory case
  And no token value is printed by these instructions

Scenario: Forget a saved server address
  Given the browser remembers a server address
  When the owner forgets it
  Then the field and saved address are cleared
  And a reload does not restore it

Scenario: Copy feedback follows the current command
  Given a command has been copied
  When two seconds pass or the operating system changes
  Then the button offers to copy the current command again
  And an earlier pending copy cannot label a replacement command as copied

Scenario: Load a memory view in the background
  Given the memory view loads in a hidden tab
  Then it describes the refresh as paused
  And it does not request memories while hidden
  When the tab becomes visible
  Then one refresh starts and normal status resumes
```

Presentation checks: README server-first order retains existing anchors; all
public mastheads and privacy footers agree; mobile navigation targets meet the
requested size; the screenshot is captured from current UI using invented
records, and its alternative text matches the image.

## Verification and delivery

- Go tests and vet across the repository pass; memory, MCP and client race
  checks pass. Local model tests without configured assets are not counted as
  real-model evidence; release CI supplies the pinned assets.
- Native Node regressions cover OS-specific archives and commands, copy feedback
  including delayed clipboard results, forgetting stored addresses, token-path
  help and hidden-tab loading. Relevant missing-behavior tests failed before
  implementation. Accepted scenarios are mapped to these native tests rather
  than a new test framework.
- Playwright CLI checked all three public pages at 375 and 1280 pixels, in light
  and dark themes: consistent navigation/privacy, targets at least 24 pixels,
  no horizontal overflow, all three OS archive URLs and actual quickstart
  clipboard contents, feedback reset, forget/focus/reload and screenshot loading.
  Twelve page/theme/size combinations passed without page-script errors.
- The 1280 by 960 screenshot is a natural browser capture of the current embedded
  view against a synthetic server. It shows Devices, tabs, filters, Live status
  and an invented preference. Pagination is below the frame and is not claimed
  in the alternative text.

Independent review found a foreground-refresh 304 status edge case; it is fixed
and covered by a failing-before/passing-after regression. All 100 Node tests
pass. Final CI, Copilot review and 2.8.1 delivery remain.
Physical devices, screen readers, Safari/Firefox and fresh native agent turns
are not established by these checks.
