# Grasshopper status

Current release: [2.10.5](https://github.com/MylesMCook/grasshopper/releases/tag/v2.10.5),
on the marketplace and usegrasshopper.com; the macOS service runs 2.10.5.
Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

Context-aware recall is released and deployed. The server balances scopes and
old/new records when startup exceeds budget; updated hooks guide sanitized
task search. See [verification](docs/verification.md) for live evidence and
the remaining client and backup verification boundaries.

2.10.4 fixed Windows hook executable resolution and moved similarity comparison
to the opened suggestion (#81). Its Windows Codex plugin remains installed;
the 2.10.5 rollout did not update installed connectors.

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
