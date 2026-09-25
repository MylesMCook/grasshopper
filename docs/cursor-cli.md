# Cursor Agent CLI: MCP tool access

**Verified path:** add Grasshopper to the project's `.cursor/mcp.json` and
approve its read tools in `.cursor/cli.json`. The [client package guide](../integrations/plugins/README.md)
provides both templates. The client still connects to the same authenticated
backend; these files only tell Cursor how to start and approve it.

In the project's Git root, run `agent mcp list` and
`agent mcp list-tools grasshopper`. A connected server lists `context`,
`store`, `search`, `get`, and `archive`. In a fresh Agent CLI turn, call
`context` once if startup context is absent. Approve writes deliberately;
the packaged `.cursor/cli.json` example pre-approves reads only.
For a headless read test on Cursor Agent CLI `2026.09.23-86fc751`, use
`agent --print --auto-review`. In fresh Mac and Beelink workspaces, default
`--print` rejected `context` despite an enabled server and the project read
allowlist; `--auto-review` completed the scoped read on both. `--mode ask`
also rejected MCP in an earlier pilot. Keep `store` and `archive` out of the
read allowlist. Do not use `--force` or `--approve-mcps` as a workaround.
Pass the stable `id:` or `git:` project scope, never the folder name.

## What the CLI test established

On macOS arm64 with Cursor Agent CLI `2026.09.23-86fc751`, a fresh Composer 2.5
turn used the project MCP config to call `grasshopper-context` once. It read
synthetic global record 1, key `shared-pilot-marker`, revision 2, with marker
`pebble atlas` from the shared loopback test backend. No memory was written.
`agent mcp list-tools grasshopper` independently listed all five tools.

The same CLI did **not** register Grasshopper MCP tools when the package was
loaded only with `agent --plugin-dir`, despite a live backend and valid client
config. The pilot was a Git-root workspace. Fresh turns returned no Grasshopper
namespace from `GetDynamicTools`.
The current Cursor Plugin manifest, default `mcp.json` discovery, the
documented `command` plus `cwd` form, and a root Agent Plugins manifest gave
the same result. This limits the tested `--plugin-dir` path; it does not prove
that a marketplace-installed plugin or Cursor IDE behaves the same way.

In a later live Mac mini test, fresh Mac and Beelink Composer 2.5 turns each
read synthetic project record 7, revision 1, with the same unseen marker via
the private service and `--auto-review`. Codex CLI on both machines fetched
that record through one MCP `context` call. The record was archived after the
test. Beelink's forced CLI sandbox could not start because of local AppArmor;
the CLI's documented default allowlist mode ran without changing host policy.

Cursor's [plugin reference](https://cursor.com/docs/reference/plugins) documents
plugin MCP components and `${CURSOR_PLUGIN_ROOT}` expansion. Its
[CLI MCP guide](https://cursor.com/docs/cli/mcp) documents project `mcp.json`
discovery and `agent mcp list-tools`. The installed CLI's marketplace command
accepted a Git URL, but offered no local plugin install command. A native
marketplace install remains an untested release gate; the project MCP path above
is the tested CLI setup.
