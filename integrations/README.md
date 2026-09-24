# Connect Codex Desktop, Cursor, or Claude Code

All three harnesses use the **same Go client** and the same [memory policy](policy/AGENTS.md). Keep each client configuration local and untracked. No installer changes global settings, another repository, or managed policy.

**Status:** The Go client passed synthetic checks on Mac, Work HP Windows, and Beelink Linux. Fresh Mac CLI sessions checked shared reads and corrections. The current Codex pilot hook is trusted; a Mac Claude Code 2.1.280 probe needed the AGENTS.md hook fallback. Client plugin packages are under test; [install, update, and remove them](plugins/README.md), or use the manual setup below. [Observed results and open checks](../docs/memory-acceptance.md).

## 1. Prepare the client

Build `./cmd/grasshopper` or extract the matching [portable Go bundle](../docs/go-package.md). Use the absolute path to `bin/grasshopper` (macOS) or `bin/grasshopper.exe` (Windows). This client has no writable database and needs no ONNX model.

Copy `client.example.json` to a **private, untracked** path on the client machine. Set an approved private HTTPS `/mcp` URL, a stable device ID, and the absolute path to this checkout’s `policy/AGENTS.md`. Choose exactly one credential reference: `token_env` or `token_file`. Keep the token outside Git and arguments. On macOS a token file must be owner-only (`chmod 600`); on Windows restrict its ACL to the intended user and trusted admins. The backend requires a token even behind a proxy.

The client uses normalized Git `origin` for project identity, preserving path case and dropping credentials. Set a project-local `grasshopper.project-id` only when a durable override is intentional. With neither a remote nor an ID, project scope stays unresolved; the client loads only applicable global/device/OS context.

## 2. Wire one harness

Merge only the Grasshopper entries from the examples below. Replace both placeholders with absolute paths. Preserve existing entries, unrelated hooks, and managed settings. These files are integration wiring, **not** another home for standing instructions.

| Harness | Project-local files | Check in the real app |
|---|---|---|
| Codex Desktop | Merge `codex/config.toml.example` into `.codex/config.toml`, and `codex/hooks.json.example` into `.codex/hooks.json`. | Trust the project, then review and trust the exact hook in `/hooks`. Native root AGENTS.md loads before work. The hook loads memory on start, resume, compaction, and subagent start; if it is absent, call `grasshopper/context` once. CLI success does not prove Desktop behavior. |
| Cursor | Merge `cursor/mcp.json.example` into `.cursor/mcp.json`, and `cursor/hooks.json.example` into `.cursor/hooks.json`. | Open the intended folder. In Customize → MCPs, enable the project source and reload `grasshopper`. “Local: Connected” alone does not prove Agent can use it. Cursor’s startup hook can race the first turn; use one MCP context fallback. |
| Claude Code | Merge `claude/mcp.json.example` into `.mcp.json`, and `claude/settings.json.example` into `.claude/settings.json`. | Inspect `/plugin` for the built-in agents-md mod and `/config` for instruction mode. Prefer native `claude-md-or-agents-md`. The start hook loads the same canonical policy, applicable AGENTS.md, and memory when native loading is unavailable. Read nested AGENTS.md before touching its files. Do not create a CLAUDE.md wrapper. |

For Claude Code, inspect legacy CLAUDE.md and rule files **through the applicable ancestor directories** before migration. The official option is `pluginConfigs["agents-md@builtin"].options.instructionFiles`; user/explicit/managed settings control it. Do not bypass managed-only mode. The Mac 2.1.280 fresh session did not load root AGENTS.md natively, although the upstream mod documents support. Its start hook delivered the root file and memory before the first turn; a separate probe read nested AGENTS.md before the nested file. Two small companion hooks read the existing user `~/.claude/AGENTS.md` directly because this headless client denied an on-demand read outside the project and Claude limits each hook message to 10,000 characters. They create no copy; if that file exceeds 17,000 bytes, the hook reports that guidance was not loaded. Check native loading again after upgrades. The hooks run at session/subagent start, not on every prompt or file tool call.

For Cursor Agent CLI:

1. From the project, run `agent mcp list`. If Grasshopper needs approval, run `agent mcp enable grasshopper`. Confirm with `agent mcp list-tools grasshopper`. Server approval is separate from tool approval.
2. Merge [the CLI permission example](cursor/cli.json.example) into `.cursor/cli.json`. It auto-approves only `context`, `get`, and `search`. Keep the required empty `deny` array.
3. Use interactive mode for corrections. An unlisted `store` prompts for one-time approval; headless mode rejects it and must report **not saved**. Avoid `--approve-mcps` in normal use because it approves every configured server.

The Mac synthetic check observed both read and write behavior through a project MCP source. On Work HP, the package's startup hook supplied context but its plugin-only MCP did not register for the CLI; a project-local MCP source and read-tool allowlist enabled a fresh read. Mac plugin-only loading likewise delivered startup context but had no registered Grasshopper MCP tool. Keep the project-local MCP setup for Cursor CLI until native plugin tool loading is proven. Recheck [Cursor's CLI configuration](https://prod.cursor.com/docs/cli/reference/configuration), [permissions](https://prod.cursor.com/docs/cli/reference/permissions), and [MCP commands](https://prod.cursor.com/docs/cli/mcp) after a client upgrade.

## 3. Verify and remove

In a fresh session, check root AGENTS.md, active global record ID/revision, project/device filtering, a user-approved synthetic correction with readback, and a stopped-backend attempt. A connected MCP server or hook log alone is not evidence that the first model turn had context. Writes count only after an ID/revision acknowledgement. Hooks read; the active agent decides what to save. Network work has a five-second client deadline; it must not block coding or loop on retries.

To remove, delete **only** Grasshopper’s MCP, hook, and Cursor CLI permission entries. Remove the local client configuration and secret reference if unused elsewhere. Disable the Cursor CLI source only if no other project uses it. Leave other settings, memories, canonical policy, and AGENTS.md files in place. No CLAUDE.md, Cursor rule file, or client database is created by setup or removal.

Official references checked during the pilot: [Codex hooks](https://developers.openai.com/codex/hooks), [Codex MCP](https://developers.openai.com/codex/mcp), [Cursor hooks](https://cursor.com/docs/hooks), [Cursor MCP](https://cursor.com/docs/mcp), [Claude hooks](https://code.claude.com/docs/en/hooks), [Claude agents-md mod](https://github.com/anthropics/claude-code/tree/main/mods/agents-md).
