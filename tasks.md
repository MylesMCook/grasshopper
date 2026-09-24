# Grasshopper release state

Owner: Codex. Mac checkout: `/Users/mylescook/Code/MylesMCook/grasshopper`, branch `main`. Work HP is CLI-only while the user works there; Beelink is shared with another host-maintenance task. Preserve unrelated `.build/` and `.playwright-cli/` folders.

## Done

- [2.0.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.0.0) published with six checked Mac, Windows, and Linux archives. Synthetic backend, CLI, package, visualizer, backup, and outage checks are recorded in [acceptance evidence](docs/memory-acceptance.md).
- Mac mini has a fresh, private Grasshopper service, dedicated encrypted R2 backups, and a Tailnet-only route. No legacy records were migrated. Off-host restore and Beelink HTTPS MCP read/write passed. A checked 2.0.1 candidate binary is running; the release is not yet published.

## Release gate

- Publish 2.0.1 with the exact private-proxy Host fix and stable-scope tool guidance. Build and check all six archives from one commit; install that checked Mac archive. Retest local/remote reads and backup recovery.
- Finish Codex, Cursor, and Claude CLI checks against the persistent endpoint across intended machines; keep Work HP non-disruptive. Current Mac Codex fallback passed on a shadow server; Mac Cursor and Claude read the live service. Windows Claude read live; Windows Codex/Cursor model probes timed out while direct hooks passed. Desktop checks are user-led if an issue appears.
- Still untested: reboot persistence, Bitwarden recovery-note sync on a second device, Cursor native Git marketplace installation, Codex compaction and independent subagent-hook delivery. Legacy-data migration remains a separate decision.
