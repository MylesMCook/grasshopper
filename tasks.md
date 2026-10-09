# Grasshopper status

Current release: [2.9.3](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.3),
a client-only release on the marketplace and usegrasshopper.com; the macOS
service runs 2.9.2. Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

Fix Windows Codex startup timeouts ([LAB-238](https://linear.app/mcook/issue/LAB-238/keep-windows-codex-startup-hooks-within-their-timeout)).
Owner: Codex, macOS; checkout `~/Code/MylesMCook/grasshopper`, branch
`codex/fix-windows-hook-timeout`, based on `d006e65`. Uncommitted: bound Git
project detection and remove a PowerShell module import from the launcher.
Before the fix, synthetic stalled Git made both startup and prompt hooks take
over ten seconds; native Windows regression now returns in 2.02 seconds.
Client, CLI and bundle tests, vet and client race checks passed on macOS.
Native Windows launcher and client regression checks passed. A scratch build
loaded real project memory for startup, prompt and subagent events in
0.79–1.50 seconds; a repeated prompt was suppressed in 0.41 seconds.
Release, Windows installation and hook trust were explicitly approved.
Full tests with real Granite, vet, 45 Node checks, three race suites, native
archive startup and private-snapshot restore/rollback passed.
Next: independent PR review and CI, publish 2.9.4, update the
Windows plugin and verify native hook discovery/execution. The installed
plugin is still 2.9.3. Live server cutover is outside this client fix.

2.9.3 is installed on the Windows PC, where a fresh Cursor IDE chat loaded
project memory at startup. Next: move the Mac's Claude Code and Codex connectors
to 2.9.3 when next used. Owner: Claude Code, Windows.

## Decisions

- Small UI and copy polish ships in one batch at most weekly (see AGENTS.md).
- The memory view uses kept memories versus agent suggestions instead of kinds
  and confirmation, keeps only the latest handoff per project, and labels
  memories by where they apply. Project archive and rename are deferred.
- Public files never name the owner's machines or employer devices.
- Cursor users who also run Claude Code rely on Cursor importing the Claude Code
  plugin; the client answers imported hooks in both formats.

## Next

Update the Linux connectors when next used; deferred: project archive and rename.
