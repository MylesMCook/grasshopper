# Grasshopper status

Current release: [2.8.5](https://github.com/MylesMCook/grasshopper/releases/tag/v2.8.5),
deployed to the macOS service, the installed macOS connectors and
usegrasshopper.com. Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

Owner: Claude Code, macOS, worktrees under `grasshopper-worktrees/`.

1. Clean the repository: remove audit reports, replace the release log with
   [verification](docs/verification.md), shorten the README, run site tests in CI.
2. Drop the `go` prefix from command and package names. Behavior unchanged.
3. Automate releases on version tags: build, checksums, GitHub release,
   marketplace branch and site deploy (Cloudflare secrets are configured).
4. Delete stale branches; turn off the unused Projects tab; add a social preview
   and a bug report template.
5. Build 2.9.0, the memory view and site redesign, from the owner-approved
   prototypes. It is the first release shipped by the automated workflow.

## Decisions

- Small UI and copy polish ships in one batch at most weekly (see AGENTS.md).
- The memory view uses kept memories versus agent suggestions instead of kinds
  and confirmation, keeps only the latest handoff per project, and labels
  memories by where they apply. Project archive and rename are deferred.
- Public files never name the owner's machines or employer devices.

## Next

Merge step 1, then start step 2.
