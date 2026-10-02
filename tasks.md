# Grasshopper status

Current release: [2.9.3](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.3),
a client-only release on the marketplace and usegrasshopper.com; the macOS
service runs 2.9.2. Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

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
