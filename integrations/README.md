# Connect Codex Desktop, Cursor, or Claude Code

All three harnesses use the **same Go client** and the same [memory policy](policy/AGENTS.md). Keep each client configuration local and untracked. No installer changes global settings, another repository, or managed policy.

**Status:** Native Go bridge and hook checks passed on Mac and Work HP Windows. A fresh Mac Cursor GUI correction and Mac/Work HP Claude Code and Codex CLI reads passed through the Go client. Codex Desktop and Work HP Cursor GUI still need a fresh Go-client pass before wider installation. [Observed results and open checks](../docs/memory-acceptance.md).

## 1. Prepare the client

Build `./cmd/grasshopper` or extract the matching [portable Go bundle](../docs/go-package.md). Use the absolute path to `bin/grasshopper` (macOS) or `bin/grasshopper.exe` (Windows). This client has no writable database and needs no ONNX model.

Copy `client.example.json` to a **private, untracked** path on the client machine. Set an approved private HTTPS `/mcp` URL, a stable device ID, and the absolute path to this checkout’s `policy/AGENTS.md`. Choose exactly one credential reference: `token_env` or `token_file`. Keep the token outside Git and arguments. On macOS a token file must be owner-only (`chmod 600`); on Windows restrict its ACL to the intended user and trusted admins. The backend requires a token even behind a proxy.

The client uses normalized Git `origin` for project identity, preserving path case and dropping credentials. Set a project-local `grasshopper.project-id` only when a durable override is intentional. With neither a remote nor an ID, project scope stays unresolved; the client loads only applicable global/device/OS context.

## 2. Wire one harness

Merge only the Grasshopper entries from the examples below. Replace both placeholders with absolute paths. Preserve existing entries, unrelated hooks, and managed settings. These files are integration wiring, **not** another home for standing instructions.

| Harness | Project-local files | Check in the real app |
|---|---|---|
| Codex Desktop | Merge `codex/config.toml.example` into `.codex/config.toml`, and `codex/hooks.json.example` into `.codex/hooks.json`. | Trust the project and review hook permissions. A fresh task must receive root AGENTS.md before work. If hook context is absent, it should call `grasshopper/context` once. Test nested guidance before edits. CLI success does not prove Desktop behavior. |
| Cursor | Merge `cursor/mcp.json.example` into `.cursor/mcp.json`, and `cursor/hooks.json.example` into `.cursor/hooks.json`. | Open the intended folder. In Customize → MCPs, enable the project source and reload `grasshopper`. “Local: Connected” alone does not prove Agent can use it. Cursor’s startup hook can race the first turn; use one MCP context fallback. |
| Claude Code | Merge `claude/mcp.json.example` into `.mcp.json`, and `claude/settings.json.example` into `.claude/settings.json`. | Inspect `/plugin` for the built-in agents-md mod and `/config` for instruction mode. Prefer native `claude-md-or-agents-md`; the hook reads the same policy and ancestor AGENTS.md. Test nested reads and resume separately. Do not create a CLAUDE.md wrapper. |

For Claude Code, inspect legacy CLAUDE.md and rule files **through the applicable ancestor directories** before migration. The official option is `pluginConfigs["agents-md@builtin"].options.instructionFiles`; user/explicit/managed settings control it. Do not bypass managed-only mode. Native nested loading can differ for attachments, compaction, and subagents, so the AGENTS.md policy requires direct scoped reads before edits. If the installed client lacks the mod, upgrade it rather than adding another instruction format.

## 3. Verify and remove

In a fresh session, check root AGENTS.md, active global record ID/revision, project/device filtering, a user-approved synthetic correction with readback, and a stopped-backend attempt. A connected MCP server or hook log alone is not evidence that the first model turn had context. Writes count only after an ID/revision acknowledgement. Hooks read; the active agent decides what to save. Network work has a five-second client deadline; it must not block coding or loop on retries.

To remove, delete **only** Grasshopper’s MCP and hook entries, then remove the local client configuration and secret reference if unused elsewhere. Leave other settings, memories, canonical policy, and AGENTS.md files in place. No CLAUDE.md, Cursor rule file, or client database is created by setup or removal.

Official references checked during the pilot: [Codex hooks](https://developers.openai.com/codex/hooks), [Codex MCP](https://developers.openai.com/codex/mcp), [Cursor hooks](https://cursor.com/docs/hooks), [Cursor MCP](https://cursor.com/docs/mcp), [Claude hooks](https://code.claude.com/docs/en/hooks), [Claude agents-md mod](https://github.com/anthropics/claude-code/tree/main/mods/agents-md).
