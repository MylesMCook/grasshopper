# Grasshopper status

Current release: [2.9.9](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.9),
on the marketplace and usegrasshopper.com; the macOS service runs 2.9.9.
Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

2.9.9 is released, deployed and running live: each opened memory shows Kind,
Applies to, Agents and Saved by, and rows lead with their kind (#66). Owner:
Claude Code, macOS. Mac, Windows PC and Windows work machine connectors are on
2.9.9; the Linux machine updates when next used. Verification and rollback
copies: [verification](docs/verification.md). Audit follow-ups on branch
`audit-followups` (unreleased): hooks survive unreadable input, the bridge
survives a bad line, a full pairing queue is named, and scp and git+ssh remotes
resolve. Search speed, export blocking and history vectors are deferred until
the store grows.

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
