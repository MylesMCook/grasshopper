# Grasshopper

Keep a few useful memories across Codex, Cursor, and Claude Code. One private server holds them; each agent connects to it.

<a id="need-a-server"></a>

## 1. Start your server

If you already have a private server, continue to [Connect an agent](#connect-an-agent).

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

An existing Tailscale Serve route must forward to `http://127.0.0.1:8106` with
public access off. Restart the server with its exact HTTPS hostname and port:

```sh
./bin/grasshopper-server --quickstart --allowed-proxy-host memory.example:443
```

Use the same `--data-dir` if you chose one. This flag allows the configured proxy
Host and HTTPS Origin. The default HTTPS port `:443` may be omitted by the
browser; other ports must match explicitly. It does not change saved state or
create a private route.
Creating or changing your private route is a separate host operation. See
[service templates and token recovery](docs/operations.md).

The release executables are unsigned. After verifying the archive hash and its
trusted source, macOS may require the supported **System Settings > Privacy &
Security > Open Anyway** flow for an identified app. Follow [Apple's opening
instructions](https://support.apple.com/en-us/102445); do not disable Gatekeeper
or remove quarantine recursively. On Windows, inspect the publisher/source and
follow [Microsoft SmartScreen guidance](https://learn.microsoft.com/en-us/windows/security/operating-system-security/virus-and-threat-protection/microsoft-defender-smartscreen/)
if blocked. SmartScreen behavior for these archives has not been tested. Linux
archives retain executable permissions; verify the digest before executing.


### Archive names and source builds

The commands above use packaged archive names. The Go source directories have
different names; plain `go build ./cmd/grasshopper-go-server` creates
`grasshopper-go-server`, not `grasshopper-server`. To produce the same names as
the archives from a checkout, use:

```sh
mkdir -p bin
go build -o ./bin/grasshopper ./cmd/grasshopper
go build -o ./bin/grasshopper-server ./cmd/grasshopper-go-server
go build -o ./bin/grasshopper-backup ./cmd/grasshopper-go-backup
go build -o ./bin/grasshopper-migrate ./cmd/grasshopper-go-migrate
```

On Windows, add `.exe` to each output filename. Source builds compile binaries;
they do not fetch or package the ONNX runtime, model or tokenizer. `--quickstart`
and client `connect`/`setup` expect the extracted archive layout. For a manually
configured source-built server, provide `--db`, `--token-file`, `--onnx-library`,
`--model` and `--tokenizer`; add `--create-db` for an empty store and `--visualizer`
for its owner memory view. Use each binary's `--help` for options.

## Connect an agent

Install a connector on each machine. They all use the same server and the same memories. Then connect once and try it.

<a id="1-install-a-connector"></a>

### 2. Install a connector

#### Codex

```sh
codex plugin marketplace add MylesMCook/grasshopper --ref marketplace
codex plugin add grasshopper-macos@grasshopper-marketplace
```

For Windows or Linux, choose the matching Grasshopper plugin entry instead of the Mac entry.

#### Cursor

```sh
agent plugin marketplace add https://github.com/MylesMCook/grasshopper.git --git-ref marketplace
```

Install your OS entry in Agent CLI's Plugins menu or the IDE's **Customize → Plugins**.

#### Claude Code

```sh
claude plugin marketplace add MylesMCook/grasshopper#marketplace
claude plugin install grasshopper-macos@grasshopper-marketplace
```

For Windows or Linux, install `grasshopper-windows` or `grasshopper-linux` instead. Claude Code asks before an agent first uses a Grasshopper tool; approve the three read tools once.

Prefer to pre-approve those reads and keep the connector beside your other client files? Download the [client archive for your machine](https://github.com/MylesMCook/grasshopper/releases/latest), check it against the release's `SHA256SUMS`, extract it, and run this instead of the two commands above (it installs a separate local plugin, so use one route or the other):

```sh
./bin/grasshopper connect --agents claude --url https://your-private-server
```

On Windows PowerShell:

```powershell
.\bin\grasshopper.exe connect --agents claude --url https://your-private-server
```

Then approve the matching code in the server's memory view, as in step 3.

<a id="2-connect-once"></a>

### 3. Connect once

Ask the installed agent: **Connect Grasshopper.** If this machine already has a connection, it reuses it. Otherwise, give it your private server link once (the server, memory-view, or MCP link). It returns an approval link. Open that link in your memory view, sign in as the owner if asked, check that the device and code match, and approve. No token goes through the agent.

Your agent may ask you to approve a few things natively. Codex may ask you to trust its hook and allow network access to your private server. Cursor may ask you to enable Grasshopper MCP. On Windows Cursor Agent CLI, keep the default user-level MCP location. Setup allows Grasshopper's three read tools in Cursor Agent CLI and Claude Code; saving or archiving a memory still uses each agent's normal approval. Start a fresh session after these approvals.

To check later, ask the agent to check Grasshopper. It reports the device this machine is registered as, whether the server is reachable, and whether an approval is still waiting. If the server is offline, your saved connection stays in place; try again when it is back. Windows agents share one connection under `%USERPROFILE%\.grasshopper`; an older working connection can be moved there and the old files are kept.

<a id="3-try-it"></a>

### 4. Try it

A new server has no memories. To see it work:

1. Ask the agent to save a preference, such as "Remember that I prefer short commit messages."
2. Start a fresh session and ask what it remembers about your preferences.
3. Open your memory view and find it under **Saved**. Memories an agent saves without your confirmation are listed under **Needs review** and are not loaded at startup until you confirm them.

## Use the memory view

In your private server's memory view, sign in as the owner. The tabs are:

- **Saved**: everything active, from every project by default. Narrow by project, device or platform, or search for wording or meaning. Open a memory to read its complete text, source, exact scope, confirmation state and date, and to step through earlier revisions.
- **Needs review**: memories an agent saved without your confirmation. They are not loaded at startup until you confirm them. Handoffs are left off this list because they load at startup without confirmation.
- **Archived**: memories hidden from agents, which you can restore.
- **Startup preview**: what the server would send an agent for a chosen project, device and platform, and why any other memory would not load. It shows what the server would send, not what a running session received.

You can **Edit**, **Confirm as is**, **Archive** or **Restore** a memory. An edit stores a new revision and marks the memory confirmed; earlier revisions stay in history. If an agent changed the memory after you opened it, nothing is overwritten and you see the newer text before saving again. Only confirmed memories and handoffs load into an agent's startup context, so confirming is how you promote an observation. The page also shows the server version, its search model and how many memories it holds. Project, device and platform filters organize what you see; they are not access control.

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

Claude Code (marketplace install): update the marketplace, then the plugin. Remove it with the second pair of commands.

```sh
claude plugin marketplace update grasshopper-marketplace
claude plugin update grasshopper-macos@grasshopper-marketplace
```

```sh
claude plugin uninstall grasshopper-macos@grasshopper-marketplace
claude plugin marketplace remove grasshopper-marketplace
```

Claude Code (client archive install): extract the new client beside the old one and update it. Before uninstalling, clear its Grasshopper read allowlist, then remove the plugin in Claude Code's manager.

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

Verify any saved snapshot independently:

```sh
./bin/grasshopper-backup --verify /absolute/path/to/new-backup.db
```

Both backup and verification print record count, historical revision count,
SQLite `quick_check`, size and SHA-256. Verification is read-only and fails on a
corrupt, empty or nonprivate file, or a database with pending WAL/journal state.
Use `--source` and `--dest` to make a standalone snapshot first. Compare the digest after transferring an
off-host copy. With the server stopped, copy a verified snapshot to a new private
path (mode 0600 on Mac/Linux), verify that copy again, then start the server
against it with the retained token and model settings. Open a known memory and
an earlier revision before selecting that copy for cutover. Retain the original
state and any WAL/SHM files; reconcile accepted writes before rollback.

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

[Memory view](https://usegrasshopper.com/view/) · [Verified behavior](https://github.com/MylesMCook/grasshopper/blob/main/docs/memory-acceptance.md) · [Self-hosted operations](docs/operations.md)

The [shared memory policy](https://github.com/MylesMCook/grasshopper/blob/main/integrations/policy/AGENTS.md) governs all three agents. [Repository instructions](https://github.com/MylesMCook/grasshopper/blob/main/AGENTS.md) cover development. [Report security concerns privately](https://github.com/MylesMCook/grasshopper/security/advisories/new), not in a public issue.
