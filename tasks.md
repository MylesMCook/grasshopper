# Grasshopper release state

Owner: Codex. Mac checkout: `/Users/mylescook/Code/MylesMCook/grasshopper`, branch `main`. Work HP is CLI-only while the user works there; Beelink is shared with another host-maintenance task. Preserve unrelated `.build/` and `.playwright-cli/` folders.

## Done

- [2.0.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.0.0) published with six checked Mac, Windows, and Linux archives. Synthetic backend, CLI, package, visualizer, backup, and outage checks are recorded in [acceptance evidence](docs/memory-acceptance.md).
- Mac mini has a fresh, private Grasshopper service, dedicated encrypted R2 backups, and a Tailnet-only route. No legacy records were migrated. Off-host restore and Beelink HTTPS MCP read/write passed. The current server has a task-local proxy Host fix; the published 2.0.0 archive lacks it.

## Release gate

- Publish 2.0.1 with the exact private-proxy Host fix and regression tests. Build and check all six archives from one commit; install the checked Mac archive in place of the task-local hotfix. Retest local/remote reads and backup recovery.
- Run Codex, Cursor, and Claude CLIs against the persistent endpoint across intended machines; keep Work HP non-disruptive. Record versions and actual results. Desktop checks are user-led if an issue appears.
- Still untested: reboot persistence, Bitwarden recovery-note sync on a second device, Cursor native Git marketplace installation, Codex compaction and independent subagent-hook delivery. Legacy-data migration remains a separate decision.
