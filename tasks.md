# Grasshopper status

Current release: [2.9.3](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.3),
a client-only release on the marketplace and usegrasshopper.com; the macOS
service runs 2.9.2. Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

Fix Windows Codex startup timeouts ([LAB-238](https://linear.app/mcook/issue/LAB-238/keep-windows-codex-startup-hooks-within-their-timeout)).
Owner: Codex, macOS; checkout `~/Code/MylesMCook/grasshopper`, branch
`codex/fix-native-windows-hook-shell`, based on `6eaef5a` (PR #51).
2.9.4 passed CI and archive checks but failed native Codex execution because
the turn's selected shell was PowerShell. The cmd-only launcher was withdrawn;
the normal release workflow restored the 2.9.3 marketplace and site.
2.9.5 restores a portable encoded launcher without Join-Path, preserving the
two-second Git budget and eight-second hook limit. Both caller-shell forwarding
checks passed. A backed-up local pilot with the verified 2.9.4 client and
rendered 2.9.5 hooks completed native startup in 3.38 seconds and prompt
submission in 2.69 seconds; the model confirmed injected project context.
Release, installation and trust approval remain in effect. Next: full checks,
independent PR review and CI, publish 2.9.5, replace the pilot with its verified
package, and record native post-install acceptance. Server cutover is outside
this client fix; the running service remains 2.9.2.

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
