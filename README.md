# Grasshopper

Keep a few useful memories across Codex, Cursor, and Claude Code. One private server holds them; each agent connects to it.

## Start your server

Download the [server archive for your machine](https://github.com/MylesMCook/grasshopper/releases/latest), check its SHA256SUMS file, and extract it. From that folder, run:

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

### Claude Code

Download the [client archive](https://github.com/MylesMCook/grasshopper/releases/latest), check its SHA256SUMS file, and extract it. On the server machine, run:

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

[Memory view](https://usegrasshopper.com/view/) · [Update or remove a connector](https://usegrasshopper.com/setup/#update-remove) · [Backup and recovery](https://github.com/MylesMCook/grasshopper/blob/main/docs/shared-memory.md) · [Verified behavior](https://github.com/MylesMCook/grasshopper/blob/main/docs/memory-acceptance.md)

The [shared memory policy](https://github.com/MylesMCook/grasshopper/blob/main/integrations/policy/AGENTS.md) governs all three agents. [Repository instructions](https://github.com/MylesMCook/grasshopper/blob/main/AGENTS.md) cover development. Report security concerns privately to [mylesmcook@gmail.com](mailto:mylesmcook@gmail.com), not in a public issue.
