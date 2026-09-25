# Manual agent wiring

Use the [client package guide](plugins/README.md) for normal installation. These examples are for project-local setups or clients the installer cannot configure. They connect to the same server and [AGENTS.md memory policy](policy/AGENTS.md); they do not create another writable store or standing-instruction file.

## Prepare

Build `./cmd/grasshopper` or extract a [client archive](https://github.com/MylesMCook/grasshopper/releases/latest). Copy [client.example.json](client.example.json) to a private, untracked path. Set the server's authenticated `/mcp` URL, stable device ID, and the absolute path to the canonical policy. Set exactly one credential reference: `token_file` or `token_env`. Keep the token out of Git and command arguments. Use loopback HTTP on the server host or private HTTPS from another machine.

The client resolves project identity from the normalized Git origin, removing credentials and preserving path case. A local `grasshopper.project-id` is an explicit override. A folder without either has unresolved project scope; it does not become global.

## Wire one harness

Merge only the Grasshopper entries. Replace placeholder binary and config paths with absolute paths; preserve unrelated and managed settings.

| Harness | Examples | Check |
| --- | --- | --- |
| Codex | [MCP config](codex/config.toml.example) and [hooks](codex/hooks.json.example) | Trust the project and review its hook in `/hooks`. Check a fresh first turn; MCP connection alone is not proof. |
| Cursor | [MCP config](cursor/mcp.json.example), [hooks](cursor/hooks.json.example), and [CLI read permissions](cursor/cli.json.example) | Enable the MCP source, start a fresh Agent chat, and check a known record. The read allowlist covers only `context`, `get`, and `search`; approve writes separately. |
| Claude Code | [MCP config](claude/mcp.json.example) and [hooks](claude/settings.json.example) | Check the installed `agents-md` mod and instruction-loading mode. The hook loads the same policy when native loading is insufficient. Never add a CLAUDE.md wrapper. |

Claude Code's native AGENTS.md behavior depends on version and settings. Inspect legacy instruction files through the applicable ancestor directories before migrating them, and leave managed modes intact. The [observed loading tests](../docs/memory-acceptance.md) distinguish native loading, hook delivery, nested files, and subagents.

For Cursor Agent CLI, run `agent mcp enable grasshopper` if prompted, then `agent mcp list-tools grasshopper`. The tested headless CLI needed `agent --print --auto-review` for synthetic reads; default `--print` denied them. See the [CLI test record](../docs/cursor-cli.md). Do not use force flags to bypass write approval.

## Verify and remove

In a fresh session, check the applicable AGENTS.md, the record ID and revision delivered before tool use, project/device scope, and a synthetic correction with readback. During an outage, coding must continue within a bounded time; a timed-out write is not saved.

For removal, delete only Grasshopper's MCP, hook, and Cursor CLI permission entries. Keep the client config, token reference, and policy if another harness uses them. Removing client wiring does not delete server memories.

Official references: [Codex hooks](https://developers.openai.com/codex/hooks), [Cursor hooks](https://cursor.com/docs/hooks), [Claude hooks](https://code.claude.com/docs/en/hooks), and [Claude agents-md mod](https://github.com/anthropics/claude-code/tree/main/mods/agents-md).
