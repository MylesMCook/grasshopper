# Grasshopper release state

Owner: Codex. Mac checkout: `/Users/mylescook/Code/MylesMCook/grasshopper`, branch `main`. Work HP is CLI-only while the user works there; Beelink is shared with another host-maintenance task. Preserve unrelated `.build/` and `.playwright-cli/` folders.

## Done

- [2.0.1](https://github.com/MylesMCook/grasshopper/releases/tag/v2.0.1) is published with six checked Mac, Windows, and Linux archives from `dd15c15`; final [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36067286155) passed.
- The checked Mac server runs privately with encrypted off-host backups. After synthetic pilot cleanup, the latest off-host restore matched the source bytes, passed SQLite integrity, and retained the archived record and its prior revision. No legacy records were migrated.

## Open checks

- Work HP Codex and Cursor CLI model-turn reads remain unverified after bounded timeouts; direct hooks passed, and Claude's first turn passed. Cursor's project MCP still needs approval there. Keep testing CLI-only and non-disruptive while the user works.
- Still untested: reboot persistence, Bitwarden recovery-note sync on a second device, Cursor native Git marketplace installation, Codex compaction and independent subagent-hook delivery. Desktop checks are user-led if an issue appears. Legacy-data migration remains a separate decision.
- Memory view: default to **Any device**, offer known device IDs, and retain manual entry for new devices. Do not hard-code machine names.
