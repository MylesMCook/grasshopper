# Grasshopper status

Current release: [2.10.2](https://github.com/MylesMCook/grasshopper/releases/tag/v2.10.2),
on the marketplace and usegrasshopper.com; the macOS service runs 2.10.2.
Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

2.10.2 is released, deployed and running live (#77). Owner: Claude Code,
macOS. The Mac's connectors are on 2.10.2; the Windows PC, Windows work machine
and Linux machine update when next used. `TestWindowsHookPreservesInputAndExitStatus`
timed out once on a Windows release runner; watch for flakiness. Verification
and rollback copies: [verification](docs/verification.md).

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

Update the Windows and Linux connectors when next used. Deferred: project archive and rename.
