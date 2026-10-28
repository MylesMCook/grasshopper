# Grasshopper status

Current release: [2.11.3](https://github.com/MylesMCook/grasshopper/releases/tag/v2.11.3),
on the marketplace and usegrasshopper.com; the macOS service follows releases nightly.
Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

Updates are self-running: release on merge, a nightly `grasshopper-server
upgrade` on the macOS server that only moves forward, and clients that follow
the server's version. 2.11.3 is live on the Mac; its Claude Code and Codex
agents run the matching client (see [verification](docs/verification.md)).
Pending, owner steps: the one-time plugin update on the Windows PC, and
updating Cursor's plugin on the Mac to 2.11 (its active version is
unverified). Not done: release signing (needs a new dependency) and a
per-address pairing cap (ineffective behind the private proxy).

Context-aware recall is released and deployed. The server balances scopes and
old/new records when startup exceeds budget; updated hooks guide sanitized
task search. Codex and Claude connectors on Windows and macOS are updated;
the owner completed Cursor's Windows plugin step and confirmed 2.10.5 installed
on macOS. Direct hook checks passed; native-session coverage limits are in
[verification](docs/verification.md).

The Windows Grasshopper connector reports a verified device connection to the
2.10.5 server. The encrypted off-machine backup passed a repository check and
byte-identical restore.

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

Deferred: project archive and rename.
