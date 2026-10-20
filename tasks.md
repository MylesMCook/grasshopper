# Grasshopper status

Current release: [2.10.4](https://github.com/MylesMCook/grasshopper/releases/tag/v2.10.4),
on the marketplace and usegrasshopper.com; the macOS service runs 2.10.4.
Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

2.10.4 fixes Windows hook executable resolution and moves similarity comparison
to the opened suggestion (#81). Native Windows, real-model, package, browser,
cross-platform CI and requested review checks passed. The macOS service and
Windows Codex plugin are on 2.10.4; encrypted backups before and after cutover
passed, with 2.10.3 kept for rollback. Other connectors update when next used.

The saved Windows connection is still unreachable because the existing private
proxy has no serve configuration. Routing was left unchanged. Verification and
rollback details: [verification](docs/verification.md).

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

Restore the existing private proxy only after routing approval, then verify
Windows memory access. Deferred: project archive and rename.
