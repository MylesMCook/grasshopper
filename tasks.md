# Grasshopper release state

Owner: Codex. Mac checkout: `/Users/mylescook/Code/MylesMCook/grasshopper`, branch `main`. Beelink is shared with another host-maintenance task. Preserve unrelated `.build/` and `.playwright-cli/` folders.

## Done

- [2.0.1](https://github.com/MylesMCook/grasshopper/releases/tag/v2.0.1) is published with six checked Mac, Windows, and Linux archives from `dd15c15`; final [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36067286155) passed.
- The checked Mac server runs privately with encrypted off-host backups. After synthetic pilot cleanup, the latest off-host restore matched the source bytes, passed SQLite integrity, and retained the archived record and its prior revision. No legacy records were migrated.
- A fresh 24 September R2 backup restored byte-for-byte; SQLite integrity passed, and a separate server read current and prior revisions. The live service was not interrupted.
- The memory view now defaults to browsing active global memories across devices, offers known project and device IDs with manual entry, and explains that another device's facts are inspection context. Synthetic HTTP and Chromium checks covered scope, project isolation, and 390px layout. The live server still runs the published 2.0.1 view.

## Open checks

- Prioritize Codex across machines and Codex ↔ Cursor on the same and different machines. Work HP model-turn timeouts are not a release gate: the hooks reached the service, and no Grasshopper defect was isolated. Leave Work HP alone until the user asks for another check; desktop checks are user-led if an issue appears.
- Codex compaction and independent subagent hook delivery remain unisolated; fresh and resume behavior already passed. Cursor's native Git marketplace install remains untested. A Beelink follow-up SSH probe stopped at host-key verification, before any Grasshopper request. Do not bypass host-key checks.
- Reboot persistence, Bitwarden recovery-note sync on another device, and Mac-loss recovery remain untested. Reboot or persistent service changes require separate Host Safety approval. Legacy-data migration remains separate and unapproved.
