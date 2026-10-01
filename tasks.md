# Grasshopper status

Current release: [2.9.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.0),
deployed to the macOS service, its connectors, the marketplace and usegrasshopper.com. Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

2.9.1: the store tool lists memory_type values and says where a saved memory
applies, after a Haiku test saved a global preference to one device. Owner:
Claude Code, macOS, branch `claude/store-guidance`. Next: merge, tag, deploy.

## Decisions

- Small UI and copy polish ships in one batch at most weekly (see AGENTS.md).
- The memory view uses kept memories versus agent suggestions instead of kinds
  and confirmation, keeps only the latest handoff per project, and labels
  memories by where they apply. Project archive and rename are deferred.
- Public files never name the owner's machines or employer devices.

## Next

Update the Windows PC and Linux connectors when next used; deferred: project archive and rename.
