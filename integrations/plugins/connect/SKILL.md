---
name: connect-grasshopper
description: Connect this installed Grasshopper plugin to a private server with the bundled client.
---

Connect this plugin when the user asks. Standing memory rules live in
`policy/AGENTS.md`.

Run the bundled client first. It checks only Grasshopper's known client
configuration. Do not scan unrelated files, processes, or environment for
credentials. Never ask for or copy the server's master token.

From the plugin root (two directories above this file), run the command below.
In Cursor Agent CLI, add `--cursor-cli` to it.

```sh
bin/grasshopper{{EXE}} connect --json
```

Quote the returned `next_step` when explaining any status. It is the single
source of plain-language guidance, including `approval_pending`,
`owner_credential`, `unreachable_server`, `device_mismatch`,
`network_permission_required`, and `invalid_address`. Do not paraphrase the
status code or invent a remedy. Relay the returned link, device, and code
separately when present. For `invalid_address`, ask for a corrected server link;
do not treat it as a saved-configuration conflict.

If status is `missing_address`, ask once for the private server link. A base,
memory-view, or `/mcp` link works. Then run `connect --json --url ADDRESS`.
Relay the `approval_url`, device, and code as soon as `approval_pending` appears.
The owner opens that link in their already-connected memory view and approves
the matching code. After they approve, run `connect --json` again with the same
plugin. This checks the pending request once and finishes setup. If it is still
pending, show the same link and wait for the owner; do not start another request
or poll in a loop. Approval expires after five minutes.
Do not describe a pending or unacknowledged connection as complete.
A read-only `check --json` reports `approval_ready` when approval has succeeded;
run `connect --json` once to finish that existing request. Checks never install
wiring or replace credentials.

`credential_role`, `registered_device`, `server`, and `registration` come from
the host. `owner_credential` means memory access works but this device is not
paired: explain that distinction and offer `connect --reconnect` for explicit
owner-approved pairing. `device_mismatch` needs inspection; never rewrite the
local device ID or switch servers to make the check pass. Older hosts report
registration as `unverified` while retaining memory access.

Windows default connections use `%USERPROFILE%\.grasshopper`, shared by
packaged and ordinary agents. Explicit configuration overrides remain
unchanged. Explicit Connect can move a verified legacy device connection to
that shared location while retaining the original files. Do not hunt through
package directories, copy owner credentials, or select between conflicts.


For `network_permission_required`, use the harness's normal approval to let
this command reach the private server, then retry once. Do not disable its
sandbox or broaden network access silently. For `unreachable_server`, stop and
keep the existing connection. For
`authentication_rejected`, offer `connect --reconnect` only after the owner asks
to replace access. For `conflicting_configuration`, show the next step; a server
switch requires the owner's explicit choice and `--switch-server --url ADDRESS`.
Denial and expiry require a new owner-initiated attempt. An old server without
viewer pairing needs an update; never fall back to token transfer.

For Cursor Agent CLI, `--cursor-cli` puts the MCP entry in the user's
`.cursor` directory by default; the plugin supplies the hook. Keep this
default on Windows, where project-local MCP approval can fail in Agent CLI.
Use `--cursor-dir` only when the user deliberately needs another location.

Run:

```sh
bin/grasshopper{{EXE}} check --json
```

The CLI status is the connection check. Review Codex hooks and approve Cursor
MCP when prompted, then start a fresh session. Native permissions remain the
user's decision; do not bypass them.
