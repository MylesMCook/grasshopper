# Connect an agent

One client connects Codex, Cursor, and Claude Code to your private Grasshopper server. It stores no memories locally. Use the [server setup](https://usegrasshopper.com/setup/) first.

## Connect this machine

1. Download its [client archive](https://github.com/MylesMCook/grasshopper/releases/latest), [check `SHA256SUMS`](https://github.com/MylesMCook/grasshopper/blob/main/docs/go-package.md), and extract it to a folder you will keep. Plugin paths break if you move the folder.
2. Have the server's `/mcp` address and a private token file on **this** machine. The server's `--quickstart` prints the token-file path. Transfer a copy securely to other machines; keep it out of Git and command arguments. Use loopback HTTP on the server host or private HTTPS from another machine.
3. From the extracted client folder, run the commands for your OS. Replace the address, token path, and device name.

Mac or Linux:

```sh
./codex/plugins/grasshopper/bin/grasshopper configure \
  --url https://your-private-server/mcp \
  --token-file /absolute/private/token-file \
  --device my-machine
./codex/plugins/grasshopper/bin/grasshopper check
```

Windows PowerShell:

```powershell
.\codex\plugins\grasshopper\bin\grasshopper.exe configure --url https://your-private-server/mcp --token-file C:\absolute\private\token-file --device my-pc
.\codex\plugins\grasshopper\bin\grasshopper.exe check
```

`configure` writes one private client config and the package's canonical `AGENTS.md` policy. It stores the **path** to the token, not its contents. `check` makes one bounded authenticated read and prints no memories. If it fails, fix the address, token file, or private route before adding an agent. Repeat this step on each agent machine.

## Add the agents you use

Run these from the extracted client folder.

**Codex CLI and Desktop**

```sh
codex plugin marketplace add ./codex
codex plugin add grasshopper@grasshopper-local
```

In a fresh task, use `/hooks` to review and trust the Grasshopper startup hooks. Codex skips new hooks until trusted.

**Cursor Agent CLI and IDE**

```sh
./cursor/plugins/grasshopper/bin/grasshopper cursor install
agent mcp enable grasshopper
```

On Windows, use `.\cursor\plugins\grasshopper\bin\grasshopper.exe cursor install`. Approve the Grasshopper MCP source when prompted.

**Claude Code**

```sh
claude plugin marketplace add ./claude
claude plugin install grasshopper@grasshopper-local
```

Review any prompt from Claude Code. All three agents need to be installed and signed in first.

The Cursor installer merges only Grasshopper's MCP and startup-hook entries into your user configuration. It preserves other entries and stops on a conflicting Grasshopper entry. `agent mcp enable` approves the MCP source; it does not preapprove writes. All three agents use the same config and server. They do not create Claude or Cursor rule files.

## Check a fresh session

Ask the agent which Grasshopper record it received **before tool use**, including its ID and revision. A connected MCP source or successful hook alone does not prove context reached the first turn. Test a correction with a synthetic record, then read both revisions. If the server is down, the agent should continue promptly and must not claim an unacknowledged write was saved.

Cursor's tested headless CLI may deny a read without `agent --print --auto-review`; see the [Cursor test record](../../docs/cursor-cli.md). Desktop and IDE first-turn behavior needs its own check. [Observed results](https://github.com/MylesMCook/grasshopper/blob/main/docs/memory-acceptance.md).

## Update or remove

For an update, keep the old folder until the new one passes a fresh-session check. Extract the new archive to a permanent folder and repeat `configure` with the same values plus `--update`. Then update only the agents you use:

- **Codex:** `codex plugin remove grasshopper@grasshopper-local`, then `codex plugin marketplace remove grasshopper-local`. Run the two Codex install commands above from the new folder and review changed hooks.
- **Cursor:** run the new folder's `grasshopper cursor install --update`. It moves Grasshopper's MCP and hook paths to the new binary.
- **Claude Code:** `claude plugin uninstall grasshopper@grasshopper-local`, then `claude plugin marketplace remove grasshopper-local`. Run the two Claude install commands above from the new folder.

To remove Grasshopper, run the removal commands for your installed agents without reinstalling. For Cursor, run `grasshopper cursor remove` from the installed client folder, then `agent mcp disable grasshopper`. Keep the shared config, token file, and `AGENTS.md` while any agent still uses them. Removing a client never deletes server memories.

For custom project-local wiring, use the [manual integration examples](https://github.com/MylesMCook/grasshopper/blob/main/integrations/README.md). The separate [Git-backed Codex marketplace](https://github.com/MylesMCook/grasshopper/tree/main/plugins/grasshopper) requires a client binary on Codex's PATH; do not install both Codex plugin routes.
