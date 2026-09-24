# Grasshopper after 2.0.0

Owner: Codex. Mac checkout: `/Users/mylescook/Code/MylesMCook/grasshopper`,
branch `main`. Work HP checks remain CLI-only while the user works there.
Beelink is shared with a separate host-maintenance task. Preserve unrelated
`.build/` and `.playwright-cli/` folders.

## Released

[Grasshopper 2.0.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.0.0)
is tagged at `45a7e08`. Six Mac/Windows/Linux server and client archives plus
`SHA256SUMS` are published. Their GitHub digests match local verified files.
[Final CI](https://github.com/MylesMCook/grasshopper/actions/runs/36058234153)
passed on all three OSes.

One authenticated Go/SQLite service exposes `context`, `store`, `search`,
`get`, and `archive`. One stateless client connects Codex, Cursor, and Claude.
The optional read-only live view uses the requested design foundation. Rust
and local code search are retired. See [acceptance evidence](docs/memory-acceptance.md)
for real BGE, backup, CLI, cross-machine, package, browser, and outage checks.
All release tests used synthetic data; no live service or data changed.

## Next gates

- Mac mini is the selected first persistent host. The
  [deployment plan](docs/mac-mini-deployment.md) is prepared, not applied.
  Choose and verify an independent off-host backup, then obtain explicit
  approval for the launchd job and tailnet-only Serve route before applying.
  Rehearse live recovery and test all three CLIs through that private route.
- Cursor CLI works with project MCP wiring; native Git marketplace installation
  is untested. Codex compaction and independent subagent-hook delivery remain
  open. Desktop checks are user-led if a real issue appears.
- Do not treat a published software release as a live migration. Preserve
  unknown-scope legacy records until reviewed.
