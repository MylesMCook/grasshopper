---
name: connect-grasshopper
description: Connect this installed Grasshopper plugin to a private server with the bundled client.
---

Connect this plugin when the user asks. Standing memory rules live in
`policy/AGENTS.md`.

Get the private server's `/mcp` address and the absolute path to an existing
token file. Never ask for or print the token. Use loopback defaults only when
they match the running server.

From the plugin root (two directories above this file), run
`bin/grasshopper{{EXE}} setup --agents none --url ADDRESS --token-file PATH`.
It checks authentication before saving config. Use `--update` only to replace
an existing Grasshopper config. If it fails, report the error; do not claim
the connection worked.

For Cursor Agent CLI, add `--cursor-cli`. It puts the MCP entry in the user's
`.cursor` directory by default; the plugin supplies the hook. Keep this
default on Windows, where project-local MCP approval can fail in Agent CLI.
Use `--cursor-dir` only when the user deliberately needs another location.

Run `bin/grasshopper{{EXE}} check`, then look for a known memory in a fresh
session before tool use. Review Codex hooks and approve Cursor MCP when
prompted. Config alone does not prove first-turn context.
