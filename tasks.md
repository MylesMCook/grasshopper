# Grasshopper status

Current release: [2.9.1](https://github.com/MylesMCook/grasshopper/releases/tag/v2.9.1),
deployed to the macOS service, its connectors, the marketplace and usegrasshopper.com. Earlier release records are in Git history and
[GitHub releases](https://github.com/MylesMCook/grasshopper/releases).

## Active

[LAB-237](https://linear.app/mcook/issue/LAB-237/show-each-agents-grasshopper-version-in-check):
`check` lists each agent's connector version against the server. Implemented on
branch `check-agent-versions`; tests pass and verified on a Windows PC (see
`docs/verification.md`). PR open; awaiting CI and one requested review. Owner: Claude Code, Windows.

## Decisions

- Small UI and copy polish ships in one batch at most weekly (see AGENTS.md).
- The memory view uses kept memories versus agent suggestions instead of kinds
  and confirmation, keeps only the latest handoff per project, and labels
  memories by where they apply. Project archive and rename are deferred.
- Public files never name the owner's machines or employer devices.

## Next

Update the Windows PC and Linux connectors when next used; deferred: project archive and rename.
