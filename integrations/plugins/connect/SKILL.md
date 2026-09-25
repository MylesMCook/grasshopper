---
name: connect-grasshopper
description: Connect this installed Grasshopper plugin to a private server with the bundled client.
---

Connect this plugin when the user asks. Standing memory rules live in
`policy/AGENTS.md`.

Ask only for the private server's `/mcp` address if it was not supplied. The
owner can copy it from **Connect another device** in their connected memory
view. Do not search this machine's files, processes, or environment for
credentials, guess localhost, ask for a token, or copy the server's master
token.

From the plugin root (two directories above this file), run:

```sh
bin/grasshopper{{EXE}} connect --url ADDRESS
```

Run it so you can relay its first output while it waits; do not hide the code
inside a long-running tool call. Tell the user to open the same server's
already connected memory viewer and approve the matching device and code.
Then wait for the command's result, for at most five minutes. A successful
connection creates a device credential locally. The server's master token
stays on the server. If an existing Grasshopper
config conflicts, inspect it before using `--update`. If the server does not
support pairing, report the required server update instead of falling back to
token transfer.

For Cursor Agent CLI, add `--cursor-cli`. It puts the MCP entry in the user's
`.cursor` directory by default; the plugin supplies the hook. Keep this
default on Windows, where project-local MCP approval can fail in Agent CLI.
Use `--cursor-dir` only when the user deliberately needs another location.

Run:

```sh
bin/grasshopper{{EXE}} check
```

Then look for a known memory in a fresh session before tool use. Review
Codex hooks and approve Cursor MCP when prompted. Config alone does not prove first-turn context.
