# Grasshopper release state

Owner: Codex · Mac mini · `main` (release code `e26a1f6`).

- [2.2.5](https://github.com/MylesMCook/grasshopper/releases/tag/v2.2.5) is public. [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36189022975) passed Mac, Linux, and Windows. Seven archives passed 280 internal checksums; published hashes match. The [marketplace](https://github.com/MylesMCook/grasshopper/tree/marketplace) is `6e39b72`.
- The [public site](https://usegrasshopper.com/) serves 2.2.5 setup links (deployment `f4d68529-3afb-4d4d-a47a-cfa53593fe37`). Home, setup, and memory view returned 200; public MCP and private visualizer routes returned 404.
- The private Mac mini server and Codex, Cursor, and Claude clients run 2.2.5 on the existing database and Tailnet route. Local and Tailnet health, scoped current/prior reads, semantic search, and the visualizer passed. Fresh CLI sessions in all three harnesses received record 10 revision 6 before tools. The release handoff was acknowledged as revision 7; the prior revision remains readable.
- Beelink independently retrieved the recovery note, restored R2, and read record 10 revision 6 from a separate loopback server. Its published 2.2.5 Linux package also passed a fresh synthetic store/get/semantic smoke. Temporary secrets, test data, and listeners were removed. A postrelease R2 backup and repository check passed; its local snapshot contains revision 7.

Still to verify: current Codex Desktop and Cursor IDE behavior, Windows 2.2.5 runtime, compaction/subagent refresh, reboot persistence, and actual host failover. Work HP was not changed in this release. See [observed behavior](docs/memory-acceptance.md); do not infer app behavior from CLI passes.
