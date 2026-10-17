# Grasshopper status

Current release: [2.9.7](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.7),
on the marketplace and usegrasshopper.com; the macOS service runs 2.9.7.
Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

2.9.8 (in release; Claude Code, macOS, branch `release-2.9.8`) ships memory view
improvements: project pages show what agents start with and link the startup
preview (#61), agent names read as products, Archived folds replaced handoffs,
a sidebar hairline under All projects, and owner-chosen device names that never
change device IDs (#62, #63). Device names add a `device_labels` table, so the
release needs the full rehearsal; live cutover waits for owner approval. 2.9.7
runs live. Remaining audit findings 10 to 20 are not started.

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

Update the Linux connector when next used. Deferred: project archive and rename,
and audit findings 10 to 20.
