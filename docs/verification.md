# Verification

What has been checked on real machines, what every release must pass, and what
has never been verified. Per-release detail lives in each
[GitHub release](https://github.com/MylesMCook/grasshopper/releases) and in Git
history (the full log up to 2.8.5 is in
[the 2.8.5 tree](https://github.com/MylesMCook/grasshopper/blob/v2.8.5/docs/memory-acceptance.md)).

## Clients verified on real machines

A fresh native CLI session received a confirmed memory in startup context before
any tool call. Each cell is the last version where that passed.

| Machine | Codex CLI | Cursor Agent CLI | Claude Code CLI |
| --- | --- | --- | --- |
| macOS (Apple silicon) | 2.3.4 | 2.3.6 | 2.3.4 |
| Linux (x64) | 2.3.4 | 2.3.4 | 2.3.4 |
| Windows (x64) | 2.6.0 | Not delivered in 2.3.3 | Not tested |

Since then, each release runs the installed macOS connectors' startup hooks and
five-tool bridges directly (latest: 2.9.2), without a fresh AI session.
Cross-machine save, correct and read back through one server passed between
macOS and Linux in 2.3.4. Codex Desktop and Claude desktop have not been
retested since 2.3.0; Cursor IDE startup hooks are covered under 2.9.2 below.

## 2.9.0 rollout (September 30)

The tag-driven release workflow built, published and deployed 2.9.0 on its
first run: native server archives on macOS, Linux and Windows, client and
marketplace archives, `SHA256SUMS`, the marketplace branch and the site. The
macOS archive matched its published digest and every embedded hash; its model
and runtime were identical to the installed ones.

Before cutover, the 2.9.0 server ran against a copy of the live database (all
active and archived memories, sessions and paired devices intact; the device
activity table was created) and 2.8.5 reopened that upgraded copy. The live
service then took a verified snapshot and an encrypted off-machine backup,
swapped in the 2.9.0 binaries under the existing file names, and restarted.
Afterwards: anonymous requests are refused, the owner view and API answer
locally and over the private HTTPS route, memory and archive counts match,
search has its model, an installed connector read a known memory, and a second
off-machine backup passed. The 2.8.5 binaries, service definition and
pre-upgrade snapshot are kept for rollback.

The macOS connectors then moved to 2.9.0: Codex through its marketplace
plugin, Claude Code and Cursor from the checksum-verified client archive with
`setup --update`. The Mac swapped the owner token for its own approved device
token; `check` reports registration verified, the Codex and Claude Code startup
hooks loaded project context with it, and its last-seen time appeared in
Agents. A fresh Claude Code session (Haiku 4.5) in an unresolved folder loaded
only the global memory, read memory 8, and saved, searched and archived a test
memory. It saved a requested global memory with device scope and guessed an
invalid `memory_type`, which 2.9.1 addresses in the `store` tool guidance.

Not verified: phones, screen readers, non-Chromium browsers, fresh AI
sessions on other machines, and last-seen times for the Windows and Linux
devices (neither has connected since the upgrade).

## 2.9.1 rollout (September 30)

The release workflow's Windows server job hit the known slow-runner test
startup timeout once and passed on rerun; every other job passed first time.
The macOS archives matched `SHA256SUMS` and every embedded hash, with the model
and runtime unchanged. The 2.9.1 server and then 2.9.0 opened the same copy of
the live database with matching counts. The live service took a verified
snapshot and an encrypted off-machine backup, swapped in the 2.9.1 binaries and
restarted. Afterwards anonymous requests are refused, the owner view answers
locally and over the private route, counts match, and `tools/list` shows the new
`store` scope guidance and `memory_type` values. The Codex, Claude Code and
Cursor connectors moved to 2.9.1 and each startup hook loaded project context
with the Mac's device token. A second off-machine backup passed. The 2.9.0
binaries, service definition and pre-upgrade snapshot are kept for rollback.

Not verified: a fresh AI session saving a global memory under the new guidance.

## 2.9.2 rollout (October 1)

The release workflow passed every job on its first run. The macOS server
archive matched `SHA256SUMS` and every embedded hash, with the model and runtime
unchanged. Rehearsal on copies: packaged `--quickstart` started, owner sign-in
returned a session, and it shut down cleanly; a restored copy of the live
database opened under 2.9.2 with owner sign-in, a record read at its current and
earlier revision, and search results; a separate copy opened by 2.9.2 reopened
under 2.9.1 with search results. The live service stopped, took a verified
snapshot (38 records, 99 revisions, 84 receipts, 5 credentials), swapped in the
2.9.2 binaries and restarted; it reports 2.9.2 over the private route, and an
encrypted off-machine backup before and after the cutover verified with the same
counts. The 2.9.1 binaries, service definition and snapshot are kept for
rollback.

On the Windows PC, Claude Code and Codex moved to 2.9.2 and `grasshopper check`
lists both as current. Replaying the exact input Cursor IDE 3.23.12 sent to an
imported hook (byte order mark, `sessionStart`) against the installed 2.9.2
plugin returned memory context in `additional_context`.

On the Mac, Claude Code (client archive) and Codex moved to 2.9.2 and `check`
lists both as current; its installed hook loaded context for both Claude's
SessionStart input and Cursor's byte-order-marked `sessionStart` input. Cursor on
both machines imports the Claude Code plugin, so the duplicate direct Cursor
wiring on the Mac was removed with `cursor remove` (other hooks kept) and
leftover Cursor plugin files were moved to the trash on both machines.

On October 2, a fresh chat in the Cursor IDE on the Mac reported the memory
loaded at startup: three confirmed global preferences, no project records and
no handoff, and it quoted one preference verbatim. The Cursor IDE version was
not recorded.

On a Windows (x64) PC, Cursor IDE 3.23.12 ran the imported Claude Code 2.9.2
plugin's sessionStart hooks in a fresh chat; all three exited 0 with
`additional_context`. With no folder open, startup memory loaded with
`project_resolved=false`. In a project folder the project hook returned only the
"startup unavailable" notice: Cursor passed the folder as
`workspace_roots: ["/C:/..."]` with no `cwd`, which the 2.9.2 client could not
resolve. Feeding that input to the 2.9.2 binary reproduced it; the same path
without the leading slash loaded project context. The client fix is in PR #48
and needs a release. The agent could still call `context` itself.

Cursor skips sessionStart for a chat typed into the panel it restores when a
window opens, or started before the window's hooks finish loading (10 to 40
seconds). Start a new chat after the window loads. A window with no folder open
loads no hooks at all.

`grasshopper check` on that PC passed with Claude Code and Codex current. Old
2.9.1 plugin copies and Cursor's Grasshopper marketplace entry were removed; if
leftover Cursor plugin files remain in its plugin cache, `check` lists them as
behind, because Cursor records no readable install state.

## 2.9.3 rollout (October 2)

The release workflow passed every job on its first run; 2.9.3 is client-only
and the macOS service stays on 2.9.2. On the Windows PC, the Claude Code and
Codex plugins moved to 2.9.3 and `grasshopper check` exited 0, listing both as
newer than the server. The installed 2.9.3 hook, fed Cursor's exact
`workspace_roots: ["/C:/..."]` input with no `cwd`, resolved the project; 2.9.2
had returned only the unavailable notice for the same input. After restarting
Cursor IDE 3.23.12 on the repository and starting a new chat once the window
had loaded, the agent, told not to call tools, quoted the startup line with the
`git:github.com/MylesMCook/grasshopper` project and `project_resolved=true`, one
project-scoped record and three global preferences.

Not verified: Codex Desktop and Claude desktop startup since 2.3.0.

## Agent versions in check (2.9.2, October 1)

A Windows build of `grasshopper check` from the `check-agent-versions` branch
ran on a Windows (x64) PC with Claude Code, Codex and Cursor connectors and a
2.9.1 server. It listed Claude Code and Codex plugins and Cursor's direct wiring
as current, and an old Cursor marketplace plugin (2.2.0) left in Cursor's plugin
cache as behind, with its update steps; it exited 0. With an unreachable server
address it still listed every agent with server version unknown and exited 1.
`check --json` output was unchanged. A missing program and a failing agent CLI
were tested only with synthetic settings in unit tests.

On the same PC, Cursor (agent CLI 2026.09.26) had pinned the Grasshopper
marketplace to the 2.5.0 commit it saw when added; `agent plugin marketplace
update` re-indexed that same commit, and only removing and re-adding it with
`--git-ref marketplace` moved the pin to 2.9.1. Cursor's Plugins menu also
showed Claude Code's 2.9.1 plugin as Imported with all five MCP tools enabled.
Whether that imported copy's startup hook delivers context in a fresh Cursor
session, and whether Cursor loads cached plugin files for a plugin that is
turned off, were not checked.

## Cursor startup hooks on Windows (2.9.2, October 1)

On a Windows (x64) PC, Cursor IDE 3.23.12 imported the Claude Code 2.9.1
plugin and ran its SessionStart hooks, but every Grasshopper hook produced no
output. Cursor's hooks log showed the input began with a UTF-8 byte order mark
and named the event `sessionStart`; the client failed with `invalid character
'﻿'`. Replaying that exact input reproduced the failure with the released
2.9.1 client, and the `cursor-imported-hook-context` build returned memory
context in both `additionalContext` and `additional_context`. Claude Code 2.x
on the same PC still loaded context from a SessionStart hook that returns both
fields. The same log showed `cursor remove` had left `~/.cursor/hooks.json`
without a `hooks` object, which Cursor rejects.

Not verified: Cursor injecting the fixed hook's context into a fresh IDE chat.
Cursor's headless CLI runs no sessionStart hooks, so it cannot test this.

## 2.9.4 Windows Codex hook checks (October 9)

2.9.4 was withdrawn as the stable release after installed native verification
failed. The cmd.exe-only forwarding fixture passed, but Codex executes hooks
through the turn's selected shell, which was PowerShell on the Windows PC.
The FOR command failed in that shell and delivered no context. The 2.9.4
release is marked as affected; the documented publication workflow restored
the 2.9.3 marketplace and site while the correction was prepared.

The source fix based on `d006e65` gives both Git project lookups one shared
two-second budget and replaces the Windows PowerShell launcher with a native
cmd.exe FOR command, avoiding cold PowerShell startup entirely.
The plugin's eight-second outer timeout is unchanged. Before the fix, a
synthetic stalled Git executable made SessionStart and UserPromptSubmit each
take over ten seconds. The native Windows (x64) regression now returns context
in 2.02 seconds for each event with honest unresolved project scope.

On macOS, the full client, CLI and bundle package tests, their vet checks,
and the client race tests passed. On Windows, cross-compiled test executables
ran the stalled-Git, same-project identity, bridge, prompt lifecycle and Cursor
workspace-root regressions successfully. A native launcher test used Codex's
cmd.exe outer quoting with spaces, apostrophes, ampersands, parentheses and
literal percent signs in the plugin path; it preserved stdin, JSON output and
exit code 7 within 0.37–0.40 seconds without emitting progress messages.
The received binaries and synthetic database fixture matched their source hashes.

A scratch client reporting version `dev` also used the real Windows connection
through the proposed Codex launcher. SessionStart, UserPromptSubmit and
SubagentStart loaded project memory in 0.44–0.48 seconds; a repeated prompt
returned the expected empty object in 0.21 seconds. This was direct hook replay,
not a new Codex Desktop chat. The installed plugin remains 2.9.3, and no hook
trust was changed during these checks. The installed Codex Desktop version is
26.1002.7124.0 and its bundled CLI is 0.162.0-alpha.2. Installation and fresh
desktop-chat acceptance remain pending.

Before publication, `go test ./...` passed with the pinned real Granite model,
`go vet ./...` passed, all 45 Node checks passed, and the memory, service and
client race suites passed. The native macOS server archive verified all 52
embedded hashes and started its extracted model/runtime. Three client archives
and the marketplace archive built locally. A verified private snapshot restored
under 2.9.4 and reopened under 2.9.2: owner sign-in, current and prior revision
reads, context and semantic search passed with memory/revision/device counts
unchanged. The source snapshot's digest stayed unchanged; the running service
was not interrupted.

## 2.9.5 native Windows correction (October 9)

The launcher again uses an encoded PowerShell command that can be invoked
from either cmd.exe or PowerShell, with string expansion instead of Join-Path.
The two-second shared Git budget and the plugin's eight-second hook timeout
are unchanged. Forwarding regressions now cover both caller shells, exact
stdin, failure propagation and spaces, apostrophes, ampersands, parentheses,
literal percent signs and exclamation marks in the installed path.
Invocation errors terminate the launcher with a failing status; a missing
client executable must not be reported as a successful empty hook.

The synthetic forwarding fixture has a 30-second cleanup guard for cold
Windows runtime startup; it does not assert production latency. Package tests
still require the eight-second configured deadline, and native acceptance
requires completed hook runs below eight seconds with context reaching the
model. The stalled-Git regression still requires context within four seconds.

Before publication, the verified installed 2.9.4 client was piloted with the
locally rendered 2.9.5 hook definition, keeping the prior file for rollback.
Codex Desktop 26.1002.7124.0's 0.162.0-alpha.2 native app-server discovered and
trusted all three hooks. A fresh ephemeral turn completed SessionStart in
2.92 seconds and UserPromptSubmit in 3.56 seconds, both with loaded project
context; the model confirmed receipt. Native forwarding checks passed for both
caller shells. Final release, installation and post-install acceptance remain
pending.

The missing-client test first reproduced a successful empty hook in both caller
shells. After invocation errors were made terminating, both cases returned a
visible failing status; the native context-delivery pilot above was rerun with
that final launcher.

The [2.9.5 publication workflow](https://github.com/MylesMCook/grasshopper/actions/runs/37980320896)
passed all native builds, packaging, publication, marketplace and site steps.
All seven downloaded archives matched their published digests, and all 284
embedded file hashes verified. The latest stable release and setup download
links point to 2.9.5; 2.9.4 remains marked as affected and prerelease.

The Windows Codex catalog refreshed and discovery materialized 2.9.5 without
stopping active connectors. Its executable and newline-normalized hook JSON
matched the published marketplace archive. Native discovery reported all three
hooks enabled and trusted at their eight-second timeout, with no hook errors.
A fresh ephemeral turn through the installed native app-server completed
SessionStart in 2.63 seconds and UserPromptSubmit in 2.37 seconds; both loaded
project context and the model confirmed receipt. The temporary 2.9.4 pilot was
removed, with original files and rollback copies retained. The live memory
server remains 2.9.2 and was not interrupted. This verifies the native runtime,
not a separately driven Codex Desktop GUI interaction.

## Every release

- `go test ./...`, `go vet ./...`, race tests and all Node tests pass locally.
- CI passes on macOS, Linux and Windows, including the real Granite model tests.
- All seven archives pass their embedded hash checks, and every published file
  matches `SHA256SUMS`.
- Packaged `--quickstart` starts, signs in and shuts down cleanly.
- A restored copy of a real database opens under the new server: current and
  earlier revisions read correctly, owner sign-in works and search returns
  results. A separate copy rolls back to the previous release.
- After the live cutover, record, revision, receipt and credential counts match
  the snapshot taken before it, and an encrypted off-host backup is verified
  before and after.
- The public site serves every asset, and each setup command copies exactly for
  all three operating systems.

## Never verified

- Physical phones, screen readers, and browsers other than Chromium.
- Reboot persistence of the server service, and full machine-loss recovery.
- Persistent Windows or Linux server deployments (only macOS runs one).
- Native context refresh after compaction or in subagents.
- macOS Gatekeeper and Windows SmartScreen behavior for the unsigned archives.

Unit tests and CI do not prove any of these. Record new real-machine results in
this file: harness and version, OS, what was observed, and what was not.
