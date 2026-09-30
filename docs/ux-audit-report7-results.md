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

Implementation and checks are in progress. Record actual results here before
release; do not infer live or physical-device behavior from unit tests.
