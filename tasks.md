# Grasshopper release state

Owner: Codex · Mac mini · main. Preparing 2.2.5 to make Cursor and Claude CLI reads part of setup.

## Released

- [2.2.4](https://github.com/MylesMCook/grasshopper/releases/tag/v2.2.4) comes from ae39163. [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36182003653) passed tests, vet, and native builds on Mac, Linux, and Windows. The real pinned model passed the new long-memory regression after failing before the fix.
- Seven rebuilt archives passed 280 internal file checksums; all eight GitHub assets matched local SHA-256 digests. Published download links returned 200. A fresh Mac server archive started on loopback, rejected anonymous health, accepted authenticated health, and made an integrity-checked SQLite backup.
- [Marketplace](https://github.com/MylesMCook/grasshopper/tree/marketplace) is fd57a20. [Public site](https://usegrasshopper.com/) is version 8f21b94d-ada3-4417-ade9-02cb1a27ec16; prior version 176b038f-3b8d-4937-bed0-b6274a8c9108 is available for rollback. Home, setup, and memory view returned 200; public MCP and private visualizer returned 404.
- Seven Markdown files remain: two required AGENTS.md files, one README, one connector skill, this status, [verification evidence](docs/memory-acceptance.md), and a short [Mac mini operator note](docs/operations.md). Server archives carry only the README and canonical memory policy as guides.

## Still to verify

The Mac mini private server runs 2.2.4 with the original database and Tailnet route. Fresh 2.2.4 Codex, Cursor, and Claude CLI turns received context; Codex corrected project record 10 to revision 6 and Cursor read both revisions. Beelink independently unlocked the recovery note, restored the latest R2 snapshot, and read that record from a separate 2.2.4 server. Temporary credentials, restored data, and test listener were removed.

2.2.5 code adds the three Grasshopper **read** permissions to Cursor and Claude CLI setup; writes stay approval-gated. Both headless read failures were reproduced, then fresh real CLI reads passed after setup. Claude also exposed a backend mismatch: `get` rejected a project record when its caller included device and OS. A regression failed first; `get` now uses the same applicable scope as context while writes still require exact stored scope. Full Go, vet, race, and site-build checks pass. Next: build and publish 2.2.5, upgrade the Mac server, then recheck packages and links. Codex Desktop and Cursor IDE latest-package checks, compaction/subagent refresh, reboot persistence, and actual failover remain unverified. Codex hook trust and Cursor MCP approval remain native security steps.
