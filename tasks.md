# Grasshopper release state

Owner: Codex · Mac mini · main.

[LAB-212](https://linear.app/mcook/issue/LAB-212): 2.3.4 startup/policy changes are implemented and released. Compact context preserves full records; fallback state follows workspace/server/session changes. Main implementation: 105fac3; marketplace: dab0aa3.

- [Release 2.3.4](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.4): three-OS CI, tests, race checks, real model tests, and eight published asset digests passed.
- Mac private service, public setup, and Mac/Beelink Codex/Cursor/Claude clients updated. Restore and browser-session continuity passed. Fresh native CLI turns received the same preference before tools; a cross-machine correction/readback passed.
- 80 matched synthetic trials reached the expected final result. Payloads shrank about 10%; whole-agent speedups were mixed. See [evidence and limits](docs/memory-acceptance.md).
- Beelink sandbox repaired with Ubuntu's official Bubblewrap profile. Read-only writes/network remain denied; host restrictions remain enabled.

Post-update encrypted off-host backup and repository check passed. Temporary synthetic servers are stopped. Final evidence and the current handoff close LAB-212.

Outside this change: Work HP, GUI checks, native compaction/subagent refresh, reboot persistence, and a full machine-loss recovery drill. Windows Cursor CLI startup remains an upstream compatibility gap; do not infer desktop behavior from CLI tests.
