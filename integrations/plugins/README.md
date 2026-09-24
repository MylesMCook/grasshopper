# Grasshopper client package

This archive contains the same stateless client for Codex, Cursor, and
Claude Code. It has no database, model, or credential. A separate private
Grasshopper server holds the memories.

## Configure the shared connection

1. Extract the archive for your OS. From its `codex/plugins/grasshopper/bin/`
   directory, run `./grasshopper config-path` on macOS/Linux or
   `.\grasshopper.exe config-path` in PowerShell. Create the reported directory.
2. Copy the archive's `client.example.json` to that location as `client.json`.
   Set your private HTTPS `/mcp` URL, stable device ID, and absolute `token_file`
   path. Keep the token outside the archive and Git; restrict file access.
3. Copy `policy/AGENTS.md` beside `client.json`. All three clients load this one
   installed policy. Keep your repository's own AGENTS.md for project guidance.

For an isolated test, set `GRASSHOPPER_CLIENT_CONFIG` to an absolute test
`client.json`. The policy defaults to `AGENTS.md` beside it. Check a synthetic
record before using real memory.

## Install or test a client

Use the matching directory in the extracted archive. `ABSOLUTE_ARCHIVE` below
means the full path to that directory, with spaces quoted.

| Client | Supported local path | What to verify |
|---|---|---|
| Codex CLI/Desktop | `codex plugin marketplace add ABSOLUTE_ARCHIVE/codex`, then `codex plugin add grasshopper@grasshopper-local`. | Review and trust `SessionStart`, `SubagentStart`, and `UserPromptSubmit` in `/hooks`. Start a fresh task and check a synthetic record before tool use. |
| Claude Code | `claude plugin marketplace add ABSOLUTE_ARCHIVE/claude`, then `claude plugin install grasshopper@grasshopper-local`. | `claude plugin validate ABSOLUTE_ARCHIVE/claude/plugins/grasshopper`, then check a fresh turn. `claude --plugin-dir ABSOLUTE_ARCHIVE/claude/plugins/grasshopper` is a temporary test load. |
| Cursor Agent CLI | `agent --plugin-dir ABSOLUTE_ARCHIVE/cursor/plugins/grasshopper` is a temporary test load. | Its startup hook delivered context in the Mac package probe, but plugin-only loading did not register its MCP server. Use the project MCP configuration below until native plugin tool loading is verified. |

On Windows, use the same commands in PowerShell with full Windows paths. The
package selects `grasshopper.exe`. On macOS/Linux, it selects `grasshopper`.
The Windows Codex prompt hook is a fallback for tested CLI versions that did
not run `SessionStart`. It loads context before the first model turn, then
skips repeat loads in that session for one hour. It uses Windows PowerShell,
which is already part of Windows. A resume within that hour may still need one
manual `context` call if memory is stale. The desktop app has not been tested
with this package.
Cursor's CLI marketplace command currently accepts a Git URL, not a local
archive. A published Git marketplace and IDE install have not been tested.

### Cursor CLI tool access

In the intended Git workspace, merge `cursor-mcp.example.json` into
`.cursor/mcp.json`, replacing both absolute placeholders with your extracted
client binary and shared config path. Merge `cursor-cli.example.json` into
`.cursor/cli.json` to allow only Grasshopper reads in headless mode. Run
`agent mcp list`; approve only Grasshopper with `agent mcp enable grasshopper`,
then confirm `agent mcp list-tools grasshopper`. In an isolated test,
`--approve-mcps` can approve all discovered servers for that invocation; do
not use it as a normal permission policy. A headless write needs an interactive
approval or a separately reviewed permission change. The session-start hook
can race the first turn, so the canonical policy asks for one `context` call
when startup memory is absent.
The [Cursor CLI test record](../../docs/cursor-cli.md) shows the working MCP
path and the tested `--plugin-dir` limit.

Loaded hooks, connected MCP, and a valid config are separate checks. In a
fresh task, report a synthetic record's ID and revision before calling tools.
Then read it through MCP, correct it with `expected_revision`, and read current
and prior revisions. If the backend is down, ordinary coding must continue
without reporting an unacknowledged write as saved.

## Update and remove

Verify the new archive's `SHA256SUMS` after extraction. Keep the old archive
until a fresh-turn and read check passes. Do not overwrite a directory in use.

- Codex: `codex plugin remove grasshopper@grasshopper-local`, then
  `codex plugin marketplace remove grasshopper-local`. Add the new archive's
  marketplace and plugin as above. For removal, stop after those two commands.
- Claude Code: `claude plugin uninstall grasshopper@grasshopper-local`, then
  `claude plugin marketplace remove grasshopper-local`. Add/install from the
  new archive for an update. For temporary `--plugin-dir` use, stop passing
  that flag.
- Cursor CLI: stop passing `--plugin-dir`. Remove only Grasshopper's entries
  from the project's `.cursor/mcp.json` and `.cursor/cli.json`; run
  `agent mcp disable grasshopper` if it was approved. A future Git marketplace
  install should use that marketplace's update/removal controls.

Keep `client.json`, its token, and the shared AGENTS.md while any client still
uses them. The server is packaged and upgraded separately; client installation
never migrates its database. No alternate standing-instruction file is created.
