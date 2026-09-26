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

This creates an empty database and a master token without overwriting existing state. Keep the server running. Open its memory view and connect once with that token. For other machines, give the server a **private HTTPS** address, such as [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve). Keep public access off and the master token on the server, out of chat and Git.

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

In your server's connected memory view, open **Connect another device** and copy the address. For Codex or Cursor, ask the agent: **Use the connect-grasshopper skill with this address: [paste address].** It shows a code. Approve the matching device and code in the same view. No server token is copied. Approve Codex hooks or Cursor MCP when prompted. On Windows Cursor Agent CLI, keep the default user-level MCP location.

Setup allows Grasshopper's three read tools in Cursor Agent CLI and Claude Code. Saving or archiving a memory still uses each agent's normal approval.

### Claude Code

Download the [client archive for your machine](https://github.com/MylesMCook/grasshopper/releases/latest), check it against the release's `SHA256SUMS`, and extract it. Run:

```sh
./bin/grasshopper connect --agents claude --url https://your-private-server/mcp
```

On Windows PowerShell:

```powershell
.\bin\grasshopper.exe connect --agents claude --url https://your-private-server/mcp
```

Approve the matching code in the server's memory view.

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

Before removing Cursor, run this from its client archive to clear Grasshopper's MCP hook and read allowlist. Then uninstall the plugin. Other Cursor settings remain.

```sh
./bin/grasshopper cursor remove
```

Claude Code: extract the new client beside the old one and update it. Before uninstalling, clear its Grasshopper read allowlist, then remove the plugin in Claude Code's manager.

```sh
./bin/grasshopper setup --agents claude --update
```

```sh
./bin/grasshopper claude remove
```

The update reuses this machine's saved server address and device token. In the memory view, you can disconnect a device without changing other agents. A device token grants access to the whole store; project scope is not access control.

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
