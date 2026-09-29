# Grasshopper

Keep a few useful memories across Codex, Cursor, and Claude Code. One private server holds them; each agent connects to it.

## Connect an agent

Install a connector on each machine. They all use the same server and the same memories.

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

Ask the installed agent: **Connect Grasshopper.** If this machine already has a connection, it reuses it. Otherwise, give it your private server link once (the server, memory-view, or MCP link). Open the approval link it returns **in your already-connected memory view**, match the device and code, and approve. No token goes through the agent. Codex may ask you to trust its hook and allow network access to your private server; Cursor may ask you to enable Grasshopper MCP. On Windows Cursor Agent CLI, keep the default user-level MCP location.

Connection checks report the host's registered device, server and approval state.
Owner access is identified separately from device registration. Windows agents
share `%USERPROFILE%\.grasshopper`; explicit configuration paths still take
precedence. An explicit Connect can reuse a verified legacy device connection
while keeping the old files recoverable. If the server is offline, keep the
connection and retry when it is available.

Setup allows Grasshopper's three read tools in Cursor Agent CLI and Claude Code. Saving or archiving a memory still uses each agent's normal approval.

### Find and inspect a memory

In your private server's memory view, sign in as the owner and choose a project or device. Search for wording or meaning, then open a result to read its complete text, source, exact scope, confirmation state, and update date. Use the revision controls to inspect earlier versions. The live list also offers **Open memory** for a large record that does not fit in its compact preview. Search and inspection do not edit memories.

### Claude Code

Download the [client archive for your machine](https://github.com/MylesMCook/grasshopper/releases/latest), check it against the release's `SHA256SUMS`, and extract it. Run:

```sh
./bin/grasshopper connect --agents claude --url https://your-private-server
```

On Windows PowerShell:

```powershell
.\bin\grasshopper.exe connect --agents claude --url https://your-private-server
```

Approve the matching code in the server's memory view.

Start a fresh session after native approvals. A new server has no memories until you ask an agent to save a preference or decision.

## Need a server?

Download a [server archive from GitHub Releases](https://github.com/MylesMCook/grasshopper/releases/latest): `darwin-arm64` for Apple silicon, `windows-amd64` for Windows x64, or `linux-amd64` for Linux x64. Verify its SHA-256 hash against `SHA256SUMS`, extract it, then run:

Mac or Linux:

```sh
./bin/grasshopper-server --quickstart
```

Windows PowerShell:

```powershell
.\bin\grasshopper-server.exe --quickstart
```

This creates an empty database and master token without overwriting existing state. Open its memory view and sign in with that token. For other machines, give the server a **private HTTPS** address, such as [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve). Keep public access off and the master token on the server.

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

On Windows, use `./bin/grasshopper-backup.exe` and Windows paths. Stop the old server and keep its archive, configuration, and database. Never run two writers. Restore a backup to a new path and check its records before using it.

**Upgrading from 2.3.x or earlier to 2.4.0 requires re-embedding.** The new [Granite model](docs/embedding-model.md) uses a different vector space. From the extracted new server archive, rehearse this command on a backup first. For cutover, run it again with the old server stopped and a destination that does not exist:

```sh
./bin/grasshopper-migrate --source /absolute/path/to/memory.db \
  --copy /absolute/path/to/memory-granite.db \
  --onnx-library ./runtime/libonnxruntime.dylib \
  --model ./models/granite-embedding-small-english-r2/model.onnx \
  --tokenizer ./models/granite-embedding-small-english-r2/tokenizer.json
```

On Linux, use `./runtime/libonnxruntime.so`. On Windows, use `grasshopper-migrate.exe`, `runtime/onnxruntime.dll`, and Windows paths; enter the command on one line in PowerShell. Keep `model.onnx_data` adjacent to `model.onnx`.

Start the new server against the verified copy with the **same** token, listener, and private route settings. For `--quickstart`, retain the old state folder and replace its `memory.db` with the verified copy while both servers are stopped. Check a known memory and earlier revision. If verification fails, stop the new server and restore the old archive and old database. Preserve any database that accepted new writes so those writes can be reconciled before rollback.

[Memory view](https://usegrasshopper.com/view/) · [Verified behavior](https://github.com/MylesMCook/grasshopper/blob/main/docs/memory-acceptance.md) · [Mac mini operator note](https://github.com/MylesMCook/grasshopper/blob/main/docs/operations.md)

The [shared memory policy](https://github.com/MylesMCook/grasshopper/blob/main/integrations/policy/AGENTS.md) governs all three agents. [Repository instructions](https://github.com/MylesMCook/grasshopper/blob/main/AGENTS.md) cover development. [Report security concerns privately](https://github.com/MylesMCook/grasshopper/security/advisories/new), not in a public issue.
