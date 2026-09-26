# Grasshopper release state

Owner: Codex · Mac mini · `codex/230-closeout` (documentation closeout).

- [2.3.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.0) is public. [Main CI](https://github.com/MylesMCook/grasshopper/actions/runs/36198401107) passed Mac, Linux, and Windows. Published packages passed their checksums.
- The [marketplace](https://github.com/MylesMCook/grasshopper/tree/marketplace) is `f0529f7`. The [site](https://usegrasshopper.com/) serves the 2.3.0 setup page (deployment `9dfcfac8-65d3-4dac-ad63-50fff3ec2243`).
- The private Mac mini service runs 2.3.0 on its existing database and Tailnet route. A restored pre-upgrade snapshot, current integrity and health, live viewer pairing/revocation, and a fresh encrypted R2 backup passed.
- Fresh Mac Codex, Cursor, and Claude Code CLI turns received a confirmed global preference before tools on their 2.3.0 clients. Work HP Codex's 2.3.0 marketplace entry paired over Tailnet; its fresh CLI turn also received that preference before tools. Work HP Cursor Agent CLI has an enabled MCP entry and read the same record on demand, but its first-turn hook context was absent in the clean-room test.

Still open: reliable Work HP Cursor first-turn injection, current Codex Desktop and Cursor IDE checks, reboot persistence, compaction/subagent refresh, and actual host failover. See [observed behavior](docs/memory-acceptance.md). Do not infer app behavior from CLI checks.
