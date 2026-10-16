# Grasshopper status

Current release: [2.9.6](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.6),
on the marketplace and usegrasshopper.com; the macOS service runs 2.9.6.
Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

2.9.6 is released, deployed and running live: design-system alignment of the
site and memory view (#54, #55, #56). Owner: Claude Code, macOS. Verification
and rollback copies: [verification](docs/verification.md). Clients stay on their
current versions until next used.

2.9.7 (in release; Claude Code, macOS, branch `release-2.9.7`) ships the nine
audit fixes from #58: corrections keep title, tags and type; moving a keyed
memory back revives it; duplicate text is a typed `duplicate_conflict`; paired
tokens cannot list owner context; pairing checks the Host; sign-in evicts the
oldest of 256 sessions; oversized AGENTS.md no longer drops hook context;
`configure`/`setup` save the `/mcp` address; config writes refuse symlinks and
fsync. Owner approved the live cutover and client updates on every machine.

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

Update the Mac and Linux connectors when next used; the Windows Claude Code and
Cursor connector update remains separate from this Codex fix. Deferred: project
archive and rename.
