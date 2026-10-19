# Grasshopper status

Current release: [2.9.10](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.10),
on the marketplace and usegrasshopper.com; the macOS service runs 2.9.10.
Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

2.10.0 is in release: the memory view audit and dogfood fixes, Add memory,
moves between projects, a stable sidebar, and search that no longer returns
every memory (calibrated meaning cutoff, filler words ignored). Owner: Claude
Code, macOS, branch `memory-view-audit-fixes`. Server, storage and search code
changed, so the full package, rehearsal and rollback checks apply before the
live cutover. Search speed, export blocking and history vectors are deferred
until the store grows.

## Decisions

- Small UI and copy polish ships in one batch at most weekly (see AGENTS.md).
- The memory view uses kept memories versus agent suggestions instead of kinds
  and confirmation, keeps only the latest handoff per project, and labels
  memories by where they apply. Project archive and rename are deferred.
- Public files never name the owner's machines or employer devices.
- Paper Mono (SIL OFL 1.1) is the mono face on the site and memory view; Geist
  Mono stays as its fallback. Tokens follow the design system's names (`--fs-*`).
- Cursor users who also run Claude Code rely on Cursor importing the Claude Code
  plugin; the client answers imported hooks in both formats.

## Next

Update the Linux connector when next used. Deferred: project archive and rename.
