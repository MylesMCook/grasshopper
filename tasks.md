# Grasshopper release state

Owner: Codex · Mac mini · main. The root AGENTS.md has a separate user edit; leave it unstaged.

## Released

- [2.2.3](https://github.com/MylesMCook/grasshopper/releases/tag/v2.2.3) comes from a771800. One formatted README now covers Codex, Cursor, and Claude Code; the separate setup guide is removed. The site and operator commands use full-size blocks. Seven Markdown files remain: two AGENTS.md files, README, the connector skill, operator and verification docs, and this record.
- [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36177899008) passed tests, vet, race checks where supported, and native builds on macOS, Linux, and Windows. Rebuilt archives passed 289 internal checksum entries; all eight uploaded assets matched their local digests. The Mac connector passed an authenticated read-only check.
- The [marketplace branch](https://github.com/MylesMCook/grasshopper/tree/marketplace) is f892074. The [public site](https://usegrasshopper.com/) is version 176b038f-3b8d-4937-bed0-b6274a8c9108; rollback version b7cbf72e-f1ba-4f89-81bb-41fd8cd67068 remains available. Home, setup, and memory view returned 200; public MCP and private viewer returned 404. All six release download links returned 200. A live 390px browser check found 16px command blocks, no inline command snippets, and no horizontal overflow.
- The private Mac mini service remains on behavior-tested 2.2.1. No restart or live-data migration occurred for this documentation release. Earlier client, cross-machine, backup-restore, and service checks are in [verification evidence](docs/memory-acceptance.md).

## Still to verify

Codex Desktop and Cursor IDE with the latest marketplace package remain user-led. Codex compaction/subagent refresh, reboot persistence, recovery-note access from another device, and full Mac-loss recovery are untested. Codex hook trust and Cursor MCP approval remain native security steps.
