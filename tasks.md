# Grasshopper status

Current release: [2.9.5](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.5),
a client-only release on the marketplace and usegrasshopper.com; the macOS
service runs 2.9.2. Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

2.9.6 aligns the public site and memory view with the Grasshopper design system:
the grasshopper favicon ([PR #54](https://github.com/MylesMCook/grasshopper/pull/54), merged),
memory view and site accessibility fixes from the design audit
([PR #55](https://github.com/MylesMCook/grasshopper/pull/55), merged), and Paper Mono as the
mono face with design-system tokens on branch `design-system-alignment`.
Owner: Claude Code, macOS. Checks, race tests and browser renders pass locally;
CI, review, the tag and the site deploy are next. The service stays 2.9.2 until
the owner approves a cutover.

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
