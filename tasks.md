# Grasshopper release state

Owner: Codex. Machine: Mac mini. Checkout: `/Users/mylescook/Code/MylesMCook/grasshopper`, branch `main`. Beelink has a separate host-maintenance task. Preserve unrelated ignored build and browser files.

## Shipped and checked

- [2.1.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.1.0) is public from `47fb666`. Seven uploaded asset digests matched the six checked archives and external `SHA256SUMS`. [Final CI](https://github.com/MylesMCook/grasshopper/actions/runs/36094500976) passed Mac, Linux, and Windows. Extracted server archives ran with isolated data on Mac mini, Beelink, and Work HP. Physical Windows client `configure`, `check`, Cursor wiring, and bounded outage passed. All test servers are stopped.
- [usegrasshopper.com/setup/](https://usegrasshopper.com/setup/) is live at Worker version `757304d5-06c1-470d-b71a-11e31ee73ac5`. Its client-guide link opens the current GitHub README. All six server/client downloads returned 200. Live browser navigation, theme, three client links, 390px width, and console checks passed; public MCP and private-view routes returned 404.
- All six tracked READMEs now have separate jobs: project entry, client setup, manual wiring, Codex Git marketplace, website operations, and font licenses. Relative links, bundled-guide links, GitHub rendering, tests, and the site dry run passed. Commit `6d3e96e` is pushed; the older 2.1.0 archive still contains its original INSTALL.md.
- The published Mac client is installed in the Grasshopper application folder. Its config points to the existing private loopback server and token file without copying the token. Fresh Codex, Cursor, and Claude Code CLI turns received confirmed global record 8 revision 1 before tools. Codex created synthetic project record 9; Cursor corrected it at revision 2; Codex read both revisions and archived it at revision 3. Active project context then omitted it. No confirmed preference was changed.
- Isolated Codex and Claude Code profiles installed and removed both an earlier task-local and the final package. Cursor updated from the earlier binary to the final path, then removed Grasshopper while retaining unrelated MCP and hooks. The real Mac installs remain active.
- The private Mac mini service still runs its earlier checked binary with encrypted off-host backups and a signed 30-day viewer session. Release 2.1.0 has **not** replaced that live binary. [Behavior and recovery evidence](docs/memory-acceptance.md).

## Next checks

- Codex hook trust and Cursor MCP approval remain native security steps. A future plugin version should be checked for changed hook commands before approving it.
- Test fresh Codex and Cursor **model turns** on Work HP with the published client when the machine is free. Package checks passed there, but no current-release model turn did. Desktop/IDE checks are user-led if an issue appears.
- Cursor's native Git marketplace path, Codex compaction and independent subagent startup, reboot persistence, recovery-note sync on another device, and Mac-loss recovery remain unverified. Legacy-data migration is separate and unapproved.
- To update the live Mac service to the 2.1.0 binary and remove its obsolete local `/about/` route, first verify a current backup and rollback binary, then get approval for that specific restart.
