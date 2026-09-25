# Grasshopper release state

Owner: Codex · Mac mini · `/Users/mylescook/Code/MylesMCook/grasshopper` · `main`. The root AGENTS.md has an unrelated user edit; leave it unstaged.

## Released

- [2.2.1](https://github.com/MylesMCook/grasshopper/releases/tag/v2.2.1) points to `c10758f`. [Final CI](https://github.com/MylesMCook/grasshopper/actions/runs/36172005751) passed on Mac, Linux, and Windows. Six rebuilt archives passed 258 internal SHA-256 entries, and all seven uploaded assets matched staged SHA-256 digests. Their native binaries match the ones exercised on physical Mac mini, Beelink Ubuntu, and Work HP Windows scratch servers. [Evidence](docs/memory-acceptance.md).
- The `marketplace` branch at `ba9f5d3` has one setup guide and OS-specific Codex/Cursor connectors. Mac changed-version Codex and Cursor updates, Windows Cursor CLI's user-level MCP correction/readback/outage, and Linux native marketplace reads passed. Codex hook trust and Cursor MCP approval still belong to the harness.
- [usegrasshopper.com](https://usegrasshopper.com/) is live at Worker version `9005ad4e-b1ac-45df-b415-3816d6031c66`; previous version `807de18c-62e6-42c2-a086-9ac5b0833b69` is the site rollback. Browser checks passed home → setup → memory view, theme, and 390px layout. All six download links returned 200; public `/mcp` and `/visualizer/` returned 404.
- The Mac mini private service now runs server 2.2.1 after an approved restart. A consistent pre-update SQLite backup and old binary/plist are staged privately. The updated service passed authenticated `grasshopper check`, anonymous-health 401, private-viewer 200, `PRAGMA quick_check=ok`, unchanged 9 records/18 revisions, and Tailnet-only route inspection. Its launchd bootstrap failed once immediately after bootout; restoring the old binary recovered service, then a second attempt with a short unload pause succeeded.
- Nine Markdown files remain: one README, one SETUP.md, two required AGENTS.md files, SECURITY.md, connector SKILL.md, operator and verification docs, and this task record. The root AGENTS.md user's edit remains unstaged.

## Open after release

- Latest marketplace Codex Desktop and Cursor IDE checks remain user-led. Codex compaction/subagent refresh, reboot persistence, recovery-note access from another device, and full Mac-loss recovery are not verified. No legacy database was migrated. Native Codex hook trust and Cursor MCP approval remain user security steps.
