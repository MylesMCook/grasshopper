# Grasshopper release state

Owner: Codex · Mac mini · `main` (2.3.1 package fix released).

- [2.3.1](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.1) fixes unreadable root-owned plugin files. [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36223607139) passed on all three OSes; seven archives and the Beelink root-owned install passed. The [marketplace](https://github.com/MylesMCook/grasshopper/tree/marketplace) is `804c8c5`.
- Mac mini still runs compatible 2.3.0 on its existing database and Tailnet route; this package fix needs no service restart. Its restored snapshot, live pairing/revocation, and encrypted R2 backup passed before this patch.
- Mac and Beelink Codex/Cursor CLI fresh turns read the same confirmed preference before tools. Mac Claude Code did too. Direct MCP and full agent timings are in [observed behavior](docs/memory-acceptance.md). Work HP remains on 2.3.0; its Cursor CLI first-turn gap remains open.

Still open: reliable Work HP Cursor first-turn injection, current Codex Desktop and Cursor IDE checks, reboot persistence, compaction/subagent refresh, and actual host failover. Do not infer app behavior from CLI checks.
