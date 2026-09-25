# Grasshopper

Keep a few useful memories across Codex, Cursor, and Claude Code. One private server holds them; each agent connects to it.

## Start your server

Download a [server archive from GitHub Releases](https://github.com/MylesMCook/grasshopper/releases/latest): `darwin-arm64` for an Apple silicon Mac, `windows-amd64` for Windows x64, or `linux-amd64` for Linux x64. The website is optional.

Compare your download's SHA-256 hash with its line in the release's `SHA256SUMS` asset, then extract it. From that folder, run:

Mac or Linux:

```sh
./bin/grasshopper-server --quickstart
```

Windows PowerShell:

```powershell
.\bin\grasshopper-server.exe --quickstart
```

This creates an empty database and a token file without overwriting existing state. Keep the server running. Agents on another machine need its **private HTTPS** MCP address and a token file on that machine. A token grants access to the whole store; project scope is not access control. Never put the token in chat or Git.

## Connect an agent

Install the connector on each machine. All connectors use the same server.

### Codex

```sh
codex plugin marketplace add MylesMCook/grasshopper --ref marketplace
codex plugin add grasshopper-macos@grasshopper-marketplace
```

For Windows or Linux, choose the matching Grasshopper plugin entry instead of the Mac entry.

### Cursor

```sh
agent plugin marketplace add https://github.com/MylesMCook/grasshopper.git --git-ref marketplace
```

Install your OS entry in Agent CLI's Plugins menu or the IDE's **Customize → Plugins**.

For Codex or Cursor, ask the agent: **Use the connect-grasshopper skill.** Give it the private MCP address and the **path** to your token file. Approve Codex hooks or Cursor MCP when prompted. On Windows Cursor Agent CLI, keep the default user-level MCP location.

Setup allows Grasshopper's three read tools in Cursor Agent CLI and Claude Code. Saving or archiving a memory still uses each agent's normal approval.

### Claude Code

Download the [client archive for your machine](https://github.com/MylesMCook/grasshopper/releases/latest), check it against the release's `SHA256SUMS`, and extract it. On the server machine, run:

```sh
./bin/grasshopper setup --agents claude
```

On Windows PowerShell:

```powershell
.\bin\grasshopper.exe setup --agents claude
```

From another machine, provide its private address and local token-file path:

```sh
./bin/grasshopper setup --agents claude --url https://your-private-server/mcp --token-file /absolute/path/to/token-file
```

Start a fresh session. Ask the agent what it received before tool use, then save one real preference or decision. A new server has no memories yet.

## Update or remove a connector

Connector changes do not delete server memories. After an update, reconnect and test a fresh session before discarding the old package.

Codex: upgrade its marketplace, then add your OS entry again. Remove that entry in the Codex plugin manager when done.

```sh
codex plugin marketplace upgrade grasshopper-marketplace
codex plugin add grasshopper-macos@grasshopper-marketplace
```

Cursor: its Git marketplace can keep an old snapshot. Remove and re-add the marketplace, then install your OS entry again in the Plugins menu. Uninstall it there when done.

```sh
agent plugin marketplace remove grasshopper-marketplace
agent plugin marketplace add https://github.com/MylesMCook/grasshopper.git --git-ref marketplace
```

Claude Code: extract the new client beside the old one, then reconnect. If the server is on another machine, repeat the same address and token-file path you used at setup. Remove Grasshopper in Claude Code's plugin manager when done.

```sh
./bin/grasshopper setup --agents claude --update
```

Remote server:

```sh
./bin/grasshopper setup --agents claude --update --url https://your-private-server/mcp --token-file /absolute/path/to/token-file
```

## Back up and update the server

Before replacing a server, run the backup tool from the **new** server archive. It makes and checks a consistent SQLite snapshot. Keep an encrypted copy away from the server.

Mac or Linux:

```sh
./bin/grasshopper-backup --quickstart
```

Windows PowerShell:

```powershell
.\bin\grasshopper-backup.exe --quickstart
```

If you chose a custom quickstart state folder, add `--data-dir` followed by its absolute path. If you started the server with `--db`, back up that database to a path that does not exist yet:

```sh
./bin/grasshopper-backup --source /absolute/path/to/memory.db --dest /absolute/path/to/new-backup.db
```

On Windows, use `./bin/grasshopper-backup.exe` and Windows paths. Stop the old server, keep its archive, then start the new one with the **same** state, token, listener, and private route settings. Check that a known memory is readable. If it fails, stop the new server and return to the old archive; never run two writers. Restore a backup to a new path and check its records before using it. For an older database, rehearse `grasshopper-migrate` on a copy before any cutover.

[Memory view](https://usegrasshopper.com/view/) · [Verified behavior](https://github.com/MylesMCook/grasshopper/blob/main/docs/memory-acceptance.md) · [Mac mini operator note](https://github.com/MylesMCook/grasshopper/blob/main/docs/operations.md)

The [shared memory policy](https://github.com/MylesMCook/grasshopper/blob/main/integrations/policy/AGENTS.md) governs all three agents. [Repository instructions](https://github.com/MylesMCook/grasshopper/blob/main/AGENTS.md) cover development. [Report security concerns privately](https://github.com/MylesMCook/grasshopper/security/advisories/new), not in a public issue.
