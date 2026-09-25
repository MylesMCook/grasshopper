# Grasshopper release state

Owner: Codex. Mac checkout: `/Users/mylescook/Code/MylesMCook/grasshopper`, branch `main`. Beelink is shared with another host-maintenance task. Preserve unrelated `.build/` and `.playwright-cli/` folders.

## Done

- [2.0.1](https://github.com/MylesMCook/grasshopper/releases/tag/v2.0.1) is published with six checked Mac, Windows, and Linux archives from `dd15c15`; final [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36067286155) passed.
- The checked Mac server runs privately with encrypted off-host backups. After synthetic pilot cleanup, the latest off-host restore matched the source bytes, passed SQLite integrity, and retained the archived record and its prior revision. No legacy records were migrated.
- A fresh 24 September R2 backup restored byte-for-byte; SQLite integrity passed, and a separate server read current and prior revisions. The live service was not interrupted.
- The memory view now browses active global memories across devices, offers known project and device IDs with manual entry, and explains cross-device context. Commit `5e19b96` is pushed; Mac, Windows, and Linux CI passed. Synthetic HTTP and Chromium checks covered scope, project isolation, and 390px layout. The Mac mini now runs this clean-commit binary on its existing private route; authenticated health, view API, and historical MCP read passed after restart. No active records remain after pilot cleanup.
- Mac mini and Beelink clones with the same Git `origin`, different paths, and different checkout commits independently resolved the same project ID. Synthetic project-only records traveled in both directions through the live service; an unrelated project saw none. Both records were archived with history intact. See [acceptance evidence](docs/memory-acceptance.md).
- Fresh Codex and Cursor CLI turns on Mac mini and Beelink independently read the same live project-only record. Cursor headless reads needed `--auto-review`; the packaged exact read allowlist passed on Mac. The record is archived with history available. A cross-clone regression test covers credential stripping, HTTPS/SSH remote normalization, and project isolation.

## Open checks

- Prioritize Codex across machines and Codex ↔ Cursor on the same and different machines. Work HP model-turn timeouts are not a release gate: the hooks reached the service, and no Grasshopper defect was isolated. Leave Work HP alone until the user asks for another check; desktop checks are user-led if an issue appears.
- Codex compaction and independent subagent hook delivery remain unisolated; fresh and resume behavior already passed. Cursor's native Git marketplace install remains untested. The later Beelink CLI pilot used a verified SSH hostname and host key.
- Reboot persistence, Bitwarden recovery-note sync on another device, and Mac-loss recovery remain untested. Reboot or persistent service changes require separate Host Safety approval. Legacy-data migration remains separate and unapproved.
- Package 2.0.2 from native Mac, Windows, and Linux CI binaries. Verify archive contents, native startup, and checksums before publishing; the published 2.0.1 archives contain the older view.
