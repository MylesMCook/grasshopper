# Grasshopper status

Current release: [2.9.5](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.5),
a client-only release on the marketplace and usegrasshopper.com; the macOS
service runs 2.9.2. Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

Windows Codex timeout fix delivered as 2.9.5 through [PR #52](https://github.com/MylesMCook/grasshopper/pull/52)
and tag `v2.9.5` at `df544d2`. Native startup and prompt hooks completed in
2.63 and 2.37 seconds with project context reaching the model. Three hooks
are enabled/trusted; seven release digests and 284 embedded hashes passed.
2.9.4 is marked as affected. The temporary pilot is removed and rollback
copies are retained. Owner: Codex, macOS; runtime and release acceptance complete.
Detailed evidence: [verification](docs/verification.md). The service stays 2.9.2.

## Decisions

- Small UI and copy polish ships in one batch at most weekly (see AGENTS.md).
- The memory view uses kept memories versus agent suggestions instead of kinds
  and confirmation, keeps only the latest handoff per project, and labels
  memories by where they apply. Project archive and rename are deferred.
- Public files never name the owner's machines or employer devices.
- Cursor users who also run Claude Code rely on Cursor importing the Claude Code
  plugin; the client answers imported hooks in both formats.

## Next

Update the Mac and Linux connectors when next used; the Windows Claude Code and
Cursor connector update remains separate from this Codex fix. Deferred: project
archive and rename.
