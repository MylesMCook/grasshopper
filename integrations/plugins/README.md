# Connect your agents

Grasshopper keeps memories on your private server. Put this small client on each machine where you use Codex, Cursor, or Claude Code. It stores a token-file **path**, not a copy of the token or a local memory database.

## First time

1. [Start your server](https://usegrasshopper.com/setup/). Download the [client archive](https://github.com/MylesMCook/grasshopper/releases/latest) for this machine, verify its [checksum](https://github.com/MylesMCook/grasshopper/blob/main/docs/go-package.md), and extract it to a folder you will keep. Installed plugins point to that folder.
2. On the **server machine**, open a terminal in the extracted client folder and run:

   ```sh
   ./bin/grasshopper setup
   ```

   Windows PowerShell: `.\bin\grasshopper.exe setup`. This checks the local server and connects Codex and Cursor. To include Claude Code, add `--agents codex,cursor,claude`. To connect only one, use `--agents codex`, `cursor`, or `claude`.
3. On **another machine**, first give the server a private HTTPS address and securely place its access token in a private file on that machine. Then run:

   ```sh
   ./bin/grasshopper setup --url https://your-private-server/mcp \
     --token-file /absolute/private/token-file
   ```

   Windows PowerShell: `.\bin\grasshopper.exe setup --url https://your-private-server/mcp --token-file C:\absolute\private\token-file`. Add `--device stable-name` if this machine's hostname may change.

Setup checks authentication **before** changing agent settings. A bad address or token leaves them untouched. It installs only the agents you select and preserves unrelated Cursor MCP servers and hooks. It does not publish your server or set up automatic startup.

Codex asks you to review and trust new startup hooks in `/hooks`. Cursor may ask you to enable its Grasshopper MCP source. These are native security steps. If the server is new, tell one agent a **real** preference to save and ask for its record ID and revision. Start a fresh session in another agent and ask which record arrived before tool use. A connected MCP server alone does not prove that first-turn context arrived.

## Update

Download and extract the new client archive to a **new** folder. Keep the old folder for rollback. Run the same `setup` command from the new folder with `--update` and the agents you use. It checks the server first, then changes only Grasshopper's agent wiring. If a step fails, it attempts to restore the previous wiring and reports any rollback failure. Verify a fresh session before deleting the old folder.

For a server update, keep its data directory and access-token file. From the new server archive, run `./bin/grasshopper-backup --quickstart` (Windows: `.\bin\grasshopper-backup.exe --quickstart`). It makes a consistent, checked local snapshot. If you used a custom state location, add `--data-dir /absolute/path`. Keep an off-host backup too. Stop the old server, then start `grasshopper-server` with the **same** `--quickstart --data-dir` and listen settings. If startup or authenticated health fails, stop it and restart the old binary against the preserved data. See the [backup and rollback guide](https://github.com/MylesMCook/grasshopper/blob/main/docs/shared-memory.md). A persistent service needs a planned maintenance window and its own approval.

## Remove

- Codex: `codex plugin remove grasshopper@grasshopper-local`, then `codex plugin marketplace remove grasshopper-local`.
- Cursor: run `grasshopper cursor remove` from the installed client folder; then `agent mcp disable grasshopper` if you used the CLI.
- Claude Code: `claude plugin uninstall grasshopper@grasshopper-local`, then `claude plugin marketplace remove grasshopper-local`.

Keep the shared config, token file, and policy while another agent uses them. Removing a client never deletes server memories. For custom wiring, use the [manual examples](https://github.com/MylesMCook/grasshopper/blob/main/integrations/README.md). The separate [Git-backed Codex marketplace](https://github.com/MylesMCook/grasshopper/tree/main/plugins/grasshopper) expects a client binary on PATH; choose one Codex route. [Observed behavior and limits](https://github.com/MylesMCook/grasshopper/blob/main/docs/memory-acceptance.md), including [Cursor CLI approval details](../../docs/cursor-cli.md).
