# Grasshopper status

Current release: [2.9.2](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.2),
deployed to the macOS service, its connectors, the marketplace and usegrasshopper.com. Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

2.9.2 is live on the macOS service, the marketplace, and the Claude Code and
Codex connectors on both machines ([LAB-237](https://linear.app/mcook/issue/LAB-237/show-each-agents-grasshopper-version-in-check)). A fresh Cursor IDE
chat on the Mac received startup memory. Open: the same check on the Windows PC.
Owner: Claude Code, Windows.

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
