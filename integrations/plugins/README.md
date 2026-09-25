# Connect your agents

One Grasshopper server holds the memories. The client archive contains a stateless bridge for Codex, Cursor, and Claude Code. Extract it to a folder you will keep; moving it later breaks local plugin paths. Check SHA256SUMS before running it.

## 1. Connect once

Have your private server's HTTPS /mcp address and an existing local token file ready. The token file must be private to your account. Grasshopper reads it when needed; setup does not copy it into a plugin or config. On the server host, --quickstart prints its path. On another machine, provision one private token file through a secure channel.

From the extracted client archive, replace the example address, token path, and stable device name:

~~~sh
./codex/plugins/grasshopper/bin/grasshopper configure \
  --url https://your-private-server/mcp \
  --token-file /absolute/private/token-file \
  --device my-mac
~~~

~~~powershell
.\codex\plugins\grasshopper\bin\grasshopper.exe configure --url https://your-private-server/mcp --token-file C:\absolute\private\token-file --device my-windows-pc
~~~

This writes one client config and installs the package's canonical AGENTS.md memory policy beside it. It prints their paths, never the token. A changed config needs an explicit --update. It does not alter repository instructions or install an agent plugin. Repeat setup on each machine that will run an agent.

Run the same binary with check before installing a harness:

~~~sh
./codex/plugins/grasshopper/bin/grasshopper check
~~~

On Windows, use .\codex\plugins\grasshopper\bin\grasshopper.exe check. It makes one bounded, authenticated context read and prints no memory content or token. If it fails, fix the URL, token file, or private route first.

## 2. Install the harness you use

Run these from the extracted archive. All three harnesses use the same client config and backend.

**Codex CLI and Desktop**

~~~sh
codex plugin marketplace add ./codex
codex plugin add grasshopper@grasshopper-local
~~~

Start a fresh task. Open /hooks, review the Grasshopper SessionStart and SubagentStart commands, and trust them. Codex skips new plugin hooks until you do. Confirm grasshopper appears under MCP tools. A connected tool alone does not prove the first turn received memory.

**Cursor Agent CLI and local IDE**

~~~sh
./cursor/plugins/grasshopper/bin/grasshopper cursor install
agent mcp enable grasshopper
~~~

The first command merges Grasshopper's MCP server and startup hook into user-level Cursor configuration while preserving other entries. The second approves only that MCP server. Start a fresh Agent session and check a known record. Conflicting Grasshopper entries stop installation; inspect them before trying cursor install --update.

The current Cursor CLI has marketplace management but no plugin-install subcommand. Its native plugin path is still being checked. The supported MCP and hook path above passed a fresh CLI turn. See the [Cursor CLI test record](../../docs/cursor-cli.md). Managed Cursor policies take priority.

**Claude Code**

~~~sh
claude plugin marketplace add ./claude
claude plugin install grasshopper@grasshopper-local
~~~

Start a new Claude Code session and check a known record. Claude's AGENTS.md and hook behavior differ from Codex and Cursor; see the [test record](https://github.com/MylesMCook/grasshopper/blob/main/docs/memory-acceptance.md) before relying on it.

On Windows, use the same steps in PowerShell, with .exe for Grasshopper and full Windows paths. Cursor CLI and Claude Code must already be installed and signed in.

## Check the connection

In a fresh session, ask which Grasshopper preference arrived before tool use. Correct a synthetic record by ID and expected revision, then use get for current and prior revisions. A successful hook does not prove the model saw context. If the backend is down, the agent should continue within a bounded time and say no write was acknowledged.

Cursor headless reads may need agent --print --auto-review on the tested CLI version. A normal headless run can deny an MCP read even when the server is connected. Keep write approval explicit. No client has a writable memory database or alternate instruction file.

## Update or remove

Keep the old archive until a fresh-session check passes with the new one. Extract the new archive into a new permanent folder, check its checksums, and run its configure command with --update and the same server, token file, and device. Then:

- Codex: run codex plugin remove grasshopper@grasshopper-local, then codex plugin marketplace remove grasshopper-local. Add the new archive's marketplace and plugin; review the new hook definition.
- Cursor: run the new archive's grasshopper cursor install --update. Its MCP and hook paths move to the new binary. Check a fresh turn before deleting the old archive.
- Claude Code: uninstall grasshopper@grasshopper-local, remove the old grasshopper-local marketplace, then add and install the new archive.

For removal, use the harness removal commands above without reinstalling. For Cursor, run grasshopper cursor remove from the installed archive and agent mcp disable grasshopper. Removal touches only Grasshopper's MCP and hook entries. Keep the shared client config, token file, and AGENTS.md while another harness uses them; remove them only after all harnesses are disconnected. Removing a client never deletes server memories.

The [Grasshopper Git marketplace](https://github.com/MylesMCook/grasshopper/tree/main/plugins/grasshopper) is a separate Codex route for people who already have the client binary on PATH. Do not install it alongside the archive's local Codex plugin.
