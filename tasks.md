# Grasshopper release state

Owner: Codex · Mac mini · main. The root AGENTS.md has a separate user edit; leave it unstaged.

## Released

- [2.2.4](https://github.com/MylesMCook/grasshopper/releases/tag/v2.2.4) comes from ae39163. [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36182003653) passed tests, vet, and native builds on Mac, Linux, and Windows. The real pinned model passed the new long-memory regression after failing before the fix.
- Seven rebuilt archives passed 280 internal file checksums; all eight GitHub assets matched local SHA-256 digests. Published download links returned 200. A fresh Mac server archive started on loopback, rejected anonymous health, accepted authenticated health, and made an integrity-checked SQLite backup.
- [Marketplace](https://github.com/MylesMCook/grasshopper/tree/marketplace) is fd57a20. [Public site](https://usegrasshopper.com/) is version 8f21b94d-ada3-4417-ade9-02cb1a27ec16; prior version 176b038f-3b8d-4937-bed0-b6274a8c9108 is available for rollback. Home, setup, and memory view returned 200; public MCP and private visualizer returned 404.
- Seven Markdown files remain: two required AGENTS.md files, one README, one connector skill, this status, [verification evidence](docs/memory-acceptance.md), and a short [Mac mini operator note](docs/operations.md). Server archives carry only the README and canonical memory policy as guides. The private Mac mini service remains on 2.2.1; no live restart or data migration occurred.

## Still to verify

Latest-package Codex Desktop and Cursor IDE behavior remains user-led. Codex compaction/subagent refresh, reboot persistence, recovery-note access from another device, and full Mac-loss recovery are untested. Codex hook trust and Cursor MCP approval remain native security steps. Thermos's last fully paired review found one remote-update instruction gap, which was corrected and parent-checked; another fresh pair could not start because the agent-thread limit was reached.
