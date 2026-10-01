# Grasshopper status

Current release: [2.9.1](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.1),
deployed to the macOS service, its connectors, the marketplace and usegrasshopper.com. Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

Releasing 2.9.2 ([LAB-237](https://linear.app/mcook/issue/LAB-237/show-each-agents-grasshopper-version-in-check)):
`check` lists each agent's connector version against the server (#43), and
Cursor on Windows loads startup memory, including through Claude Code's imported
plugin (#44). Next: tag, rehearse on database copies, cut over the macOS service,
update the Windows PC connectors, confirm Cursor startup memory in a real IDE
chat. Owner: Claude Code, Windows.

## Decisions

- Small UI and copy polish ships in one batch at most weekly (see AGENTS.md).
- The memory view uses kept memories versus agent suggestions instead of kinds
  and confirmation, keeps only the latest handoff per project, and labels
  memories by where they apply. Project archive and rename are deferred.
- Public files never name the owner's machines or employer devices.
- Cursor users who also run Claude Code rely on Cursor importing the Claude Code
  plugin; the client answers imported hooks in both formats.

## Next

Update the Windows PC and Linux connectors when next used; deferred: project archive and rename.
