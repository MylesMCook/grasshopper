# Grasshopper release state

Owner: Codex · Mac mini · `/Users/mylescook/Code/MylesMCook/grasshopper` · `main`. The root AGENTS.md has an unrelated user edit; leave it unstaged.

## Now

- **2.2.1 release candidate:** CI [passed on Mac, Linux, and Windows](https://github.com/MylesMCook/grasshopper/actions/runs/36165978676). Extracted pre-prune 2.2.1 client/server archives passed checksums on all three physical OS types; each server started in scratch, rejected anonymous health, authenticated, and made a verified SQLite backup. Mac Codex and Cursor passed changed-version plugin update tests. Windows Cursor CLI's user-level MCP workaround passed a fresh native-plugin turn, correction, readback, and bounded outage. [Evidence](docs/memory-acceptance.md).
- **Docs and site:** One README, one SETUP.md, two required AGENTS.md files, a short operator record, verification record, required connector SKILL.md, SECURITY.md, and this task file remain. Historical prose is in Git history. The public site is locally pruned and browser-checked at 390px; no deployment yet.
- **Next:** Commit the pruned source without the user's AGENTS.md edit. Rebuild all six archives and marketplace from that commit; check every checksum and bundled guide. Publish 2.2.1, deploy the site under the user's approval, and check release links and public/private route boundaries.

## Open after release

- The persistent Mac mini server still runs its earlier binary. A specific restart needs a checked backup, saved binary/plist, and approval. Do not imply the 2.2.1 archive is running there.
- Latest marketplace Codex Desktop and Cursor IDE checks remain user-led. Codex compaction/subagent refresh, reboot persistence, recovery-note access from another device, and full Mac-loss recovery are not verified. No legacy database was migrated. Native Codex hook trust and Cursor MCP approval remain user security steps.
