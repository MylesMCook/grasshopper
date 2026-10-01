# Grasshopper status

Current release: [2.8.5](https://github.com/MylesMCook/grasshopper/releases/tag/v2.8.5),
deployed to the macOS service, the installed macOS connectors and
usegrasshopper.com. Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

Owner: Claude Code, macOS, worktrees under `grasshopper-worktrees/`.

- 2.9.0: memory view and site redesign (#37, #38) and the server changes they
  need (#36). Release by tag through the release workflow, then upgrade the
  macOS service with a verified backup and a rollback copy of 2.8.5.

## Decisions

- Small UI and copy polish ships in one batch at most weekly (see AGENTS.md).
- The memory view uses kept memories versus agent suggestions instead of kinds
  and confirmation, keeps only the latest handoff per project, and labels
  memories by where they apply. Project archive and rename are deferred.
- Public files never name the owner's machines or employer devices.

## Next

Tag v2.9.0, upgrade the macOS service, record the result in docs/verification.md.
