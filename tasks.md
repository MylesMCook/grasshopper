# Grasshopper release state

Owner: Codex · Mac mini · `/Users/mylescook/Code/MylesMCook/grasshopper` · `main`. The root AGENTS.md has an unrelated user edit; leave it unstaged.

## Released

- [2.2.2](https://github.com/MylesMCook/grasshopper/releases/tag/v2.2.2) points to `e977f45`. It formats the README, moves the security contact there, and leaves eight Markdown files. [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36174404854) passed on Mac, Linux, and Windows. The rebuilt client, server, and marketplace archives passed 292 internal SHA-256 entries; all seven uploaded release assets matched local digests. The GitHub-rendered README has headings, a numbered start path, and reference links.
- [2.2.1](https://github.com/MylesMCook/grasshopper/releases/tag/v2.2.1) remains the behavior-tested baseline. Physical Mac mini, Beelink Ubuntu, and Work HP Windows scratch servers passed startup, authentication, and backup checks. [Evidence](docs/memory-acceptance.md).
- The `marketplace` branch at `cb326e4` has OS-specific Codex/Cursor connectors at 2.2.2. The Mac 2.2.2 marketplace client passed an authenticated check against the private server. Prior changed-version Codex/Cursor updates and Windows Cursor CLI correction/readback/outage passed; Codex hook trust and Cursor MCP approval remain harness steps.
- [usegrasshopper.com](https://usegrasshopper.com/) is live at Worker version `b7cbf72e-f1ba-4f89-81bb-41fd8cd67068`; previous version `9005ad4e-b1ac-45df-b415-3816d6031c66` is the site rollback. Home, setup, and memory-view pages return 200; all six 2.2.2 downloads return 200; public `/mcp` and `/visualizer/` return 404. The previous browser checks covered navigation, theme, and 390px layout; this deployment changed only setup download links.
- The Mac mini private service now runs server 2.2.1 after an approved restart. A consistent pre-update SQLite backup and old binary/plist are staged privately. The updated service passed authenticated `grasshopper check`, anonymous-health 401, private-viewer 200, `PRAGMA quick_check=ok`, unchanged 9 records/18 revisions before the release handoff, and Tailnet-only route inspection. A project-scoped handoff was then acknowledged as record 10, revision 1, and read back. Its launchd bootstrap failed once immediately after bootout; restoring the old binary recovered service, then a second attempt with a short unload pause succeeded.
- Eight Markdown files remain: one README, one SETUP.md, two required AGENTS.md files, connector SKILL.md, operator and verification docs, and this task record. The root AGENTS.md user's edit remains unstaged.

## Open after release

- Latest marketplace Codex Desktop and Cursor IDE checks remain user-led. Codex compaction/subagent refresh, reboot persistence, recovery-note access from another device, and full Mac-loss recovery are not verified. No legacy database was migrated. Native Codex hook trust and Cursor MCP approval remain user security steps.
