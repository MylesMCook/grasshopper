# Grasshopper release state

Owner: Codex · Mac mini · `main`.

- [2.3.3](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.3) is published. [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36252586838) passed on Mac, Linux, and Windows; all seven archives and the published checksums were verified. The `marketplace` branch carries 2.3.3.
- The [public site](https://usegrasshopper.com/) serves the 2.3.3 setup. The private Mac mini service runs 2.3.3 on its existing database and Tailnet route. A consistent pre-upgrade copy was restored and read through the candidate server; a post-upgrade encrypted off-host backup completed.
- Mac mini and Beelink Codex, Cursor, and Claude clients run 2.3.3. Fresh CLI turns on both machines received the same confirmed preference before tools. Work HP Codex also passed a fresh turn. Work HP Cursor authenticates and reads through MCP, but its CLI did not invoke a test project hook or receive startup context. See [observed behavior](docs/memory-acceptance.md).

Open: Windows Cursor CLI first-turn hook support, Codex Desktop and Cursor IDE checks for this version, reboot persistence, compaction/subagent refresh, and a full machine-loss recovery drill. Codex's Windows updater prints a cache-backup error even when the installed plugin reports the new version; inspect actual state after that warning. Do not infer GUI behavior from CLI checks. Next: let the owner critique normal use while tracking Cursor's upstream hook behavior.
