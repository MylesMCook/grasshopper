# Cursor Agent CLI

Connect Cursor to the same private server used by Codex and Claude Code. The
client stores a token-file path, not a token or memory database.
The [client guide](../integrations/plugins/README.md) has the setup command.

With a marketplace plugin installed, run its **connect-grasshopper** skill.
It calls `grasshopper setup --agents none --cursor-cli` after checking the
server. This adds Grasshopper to your user-level `~/.cursor/mcp.json`; the
plugin supplies the startup hook. Then run `agent mcp enable grasshopper` and
check a fresh Agent session. If startup context is absent, call `context` once.
Approve writes deliberately. A successful `agent mcp list-tools grasshopper`
alone does not prove the model can call it.

On Windows, keep the default user-level MCP location. A [Cursor CLI bug](https://forum.cursor.com/t/cursor-cli-can-see-my-mcp-servers-but-agent-sessions-still-cant-use-them/171696/11)
can make project-level `.cursor/mcp.json` show five ready tools while Agent
sessions report none. Cursor recommends a user-level MCP entry as a workaround;
Grasshopper's default setup already writes there. Do not use
`--approve-mcps` to approve every server just to make Grasshopper work.

For a headless read check, `agent --print --auto-review` worked in the tested
Mac, Linux, and Windows CLI versions. Keep `store` and `archive` out of any
automatic read allowlist. See [observed results and limits](memory-acceptance.md#marketplace-pilot-25-september)
before treating a CLI run as proof of Cursor IDE behavior.
