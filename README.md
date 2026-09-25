# Set up Grasshopper

Grasshopper keeps memories on **one private server**. Install a connector on each agent machine; connectors do not keep a second memory database.

## 1. Start the server

Download the [server archive for your machine](https://github.com/MylesMCook/grasshopper/releases/latest), verify its `SHA256SUMS`, and extract it. In that folder, run:

| Mac or Linux | Windows PowerShell |
|---|---|
| `./bin/grasshopper-server --quickstart` | `.\bin\grasshopper-server.exe --quickstart` |

The first run creates an empty database and a private token file, then prints their locations and the local memory-view address. It will not overwrite existing state. Keep the server running. For another machine to connect, give it a private HTTPS `/mcp` address and place the token in a private file **on that machine**. Never paste the token into chat or Git.

## 2. Connect an agent

**Codex:** Add the [Grasshopper marketplace](https://github.com/MylesMCook/grasshopper/tree/marketplace), then install the entry for this machine:

```sh
codex plugin marketplace add MylesMCook/grasshopper --ref marketplace
codex plugin add grasshopper-macos@grasshopper-marketplace
```

Replace `macos` with `windows` or `linux` as needed.

**Cursor:** Run `agent plugin marketplace add https://github.com/MylesMCook/grasshopper.git --git-ref marketplace`. In Agent CLI, open `/plugins` and install your OS entry. In the IDE, use **Customize → Plugins**.

Then ask the agent: **Use the connect-grasshopper skill.** Give it the server's `/mcp` address and the *path* to the token file on this machine. The skill checks authentication before saving Grasshopper-only settings. For Cursor Agent CLI on Windows, keep its default user-level MCP location; project-level MCP approval can fail on the tested CLI version.

**Claude Code:** Download the [client archive](https://github.com/MylesMCook/grasshopper/releases/latest) for this machine, check `SHA256SUMS`, and extract it. Run `./bin/grasshopper setup --agents claude` (`.\bin\grasshopper.exe setup --agents claude` on Windows). For a remote server, add `--url https://your-private-server/mcp --token-file /absolute/path/to/token-file`. Add `--device stable-name` if the hostname may change. The same package can configure Codex or Cursor with `--agents codex,cursor` when the marketplace is unavailable.

Review Codex hooks in `/hooks` and enable Grasshopper MCP in Cursor if prompted. Start a **fresh** session and ask which known memory arrived before tool use. If none did, call `context` once. A new server has no memories until you save one explicit preference or decision.

## Update or remove

- **Codex:** `codex plugin marketplace upgrade grasshopper-marketplace`, then repeat `codex plugin add` for your OS entry. Remove with `codex plugin remove ENTRY@grasshopper-marketplace`, then `codex plugin marketplace remove grasshopper-marketplace`.
- **Cursor Agent CLI:** Uninstall Grasshopper in `/plugin list`, remove the marketplace with `agent plugin marketplace remove grasshopper-marketplace`, then add it again and reinstall your OS entry. Its `marketplace update` can report success while keeping an old Git snapshot. Remove the MCP entry with `agent mcp disable grasshopper` if you enabled it.
- **Claude Code or manual package:** Extract the new client beside the old one and repeat `grasshopper setup --agents claude --update` with the same server flags. Keep the old folder until a fresh session passes. Remove with `claude plugin uninstall grasshopper@grasshopper-local`, then `claude plugin marketplace remove grasshopper-local`.

After any connector update, run its connect skill with `--update` or the new package's `setup --update`, then `grasshopper check` and a fresh-session read. Removing a connector does **not** delete server memories. [Back up and update the server](https://github.com/MylesMCook/grasshopper/blob/main/docs/shared-memory.md) separately.
