# Verification

What has been checked on real machines, what every release must pass, and what
has never been verified. Per-release detail lives in each
[GitHub release](https://github.com/MylesMCook/grasshopper/releases) and in Git
history (the full log up to 2.8.5 is in
[the 2.8.5 tree](https://github.com/MylesMCook/grasshopper/blob/v2.8.5/docs/memory-acceptance.md)).

## 2.10.4 verification and rollout (October 20)

On Windows x64, Go 1.27.1 passed `go test ./...` with the pinned Granite and
ONNX assets, including real-model tests and native archive quickstart; vet,
all five command builds, 74 memory-view tests and 31 site tests passed.
The test process used an isolated temporary directory to keep a large
machine-level AGENTS.md out of synthetic hook fixtures, and Git's existing
shell directories were added only to its PATH.

Native cmd and Windows PowerShell tests first reproduced the old lookup with
a harmless copied test executable in the working directory or PATH. The
absolute system launcher ignored both fixtures while preserving input,
arguments, unusual plugin paths, missing-client errors and exit status.
PowerShell 7 also passed an independent harmless launcher control. This does
not establish which working directory or sandbox a real Codex session uses.

In Chromium on Windows, synthetic snapshots of 4,000 records (half kept,
half suggestions) exercised project, All projects and New views. List redraws
made zero similarity calls, versus 2,000 before the change. In one measured
run, their redraw times changed from 4,070/4,267/3,892 ms to 275/324/160 ms.
Opening a suggestion still showed its exact similar memory; Keep by keyboard,
Discard, Keep and replace, refresh, Back and Escape worked. No horizontal
overflow appeared at widths 375, 768, 1280 or 1440.

These timings cover the removed comparison work, not all large-list cost:
synthetic route/panel interactions still produced long tasks up to 2,635 ms.
No Expect or axe audit, other browser or fresh native AI session is claimed.

[PR 81](https://github.com/MylesMCook/grasshopper/pull/81) passed final-head
CI on all three systems and a requested Bugbot review with no findings.
Post-merge CI and every tagged release job passed at `27b44d4`. All seven
published archives passed their outer and embedded hashes. The published macOS
archive passed all 53 embedded hashes, packaged quickstart sign-in and clean
shutdown. On a private same-host database copy, 2.10.4 passed owner sign-in,
current and historical reads, search and the expected embedded UI hash;
2.10.3 then reopened the same copy with those controls passing.

Cutover: verified encrypted off-host backup, stopped-service snapshot, binary
swap and restart. The macOS service reports 2.10.4; authenticated loopback
health and the exact embedded UI hash pass, anonymous health is rejected,
and counts remain 275 records, 579 revisions, 367 receipts and 7 credentials.
The second encrypted off-host backup and repository check passed. The snapshot,
2.10.3 binaries and launch definition are retained for rollback.

On Windows, Codex CLI 0.162.1 upgraded only the Grasshopper marketplace/plugin
to 2.10.4. Its saved connection and device credential stayed byte-identical.
The installed hook JSON passed four native cmd/PowerShell working-directory
and PATH shadow controls, preserving arguments, input and exit status; all
three events share that launcher. A synthetic 140 KB prompt returned `{}`
with exit 0 and the expected skip diagnostic. Active sessions were not restarted.
All 20 public site assets returned HTTP 200 and setup.js advertises 2.10.4.

The saved Windows connection remains unreachable: the existing private proxy
has no serve configuration. Routing and credentials were not changed; fresh
end-to-end Windows memory access remains unverified.

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

## 2.9.6 rollout (October 16)

2.9.6 aligns the public site and memory view with the design system: the
grasshopper favicon and app icon (#54), accessibility fixes from the design
audit (#55), and Paper Mono with design-system tokens (#56). Local checks
before merge: `go test ./...`, `go vet ./...`, race tests, 31 site and 33
memory view Node tests. Playwright renders of every site page at 1280 and
390px in both themes showed no overflow or contrast failures, and the memory
view's components rendered pixel-identical to the design system.

The release workflow passed every job on its first run and deployed the site.
Live, every page and asset returned 200 with its content type, including
`fonts/paper-mono.woff2` and the three OFL notices; `/mcp` and `/visualizer/`
returned 404; the setup page linked the 2.9.6 archive and quickstart command
for each operating system, and Paper Mono and Newsreader loaded with no console
errors.

The macOS server archive matched `SHA256SUMS` and all 53 embedded hashes, with
the model and runtime byte-identical to 2.9.2. Rehearsal on copies of the live
database: 2.9.6 passed authenticated health, anonymous rejection, a record read
at its current and earlier revision, owner context, search and the new assets;
the same copy reopened under 2.9.2 with every core check passing; packaged
`--quickstart` started, signed in with a session cookie and shut down cleanly.

Cutover: an encrypted off-machine backup verified first, the service stopped,
a verified snapshot was taken (270 records, 569 revisions, 360 receipts,
5 credentials), the server, backup and migrate binaries were swapped and the
service restarted. It reports 2.9.6 on loopback and over the private route and
passes the same checks; a second encrypted off-machine backup after the cutover
has identical counts. The 2.9.2 binaries, service definition and snapshot are
kept for rollback. Clients were not updated in this release.

## 2.9.7 rollout (October 16)

2.9.7 fixes nine findings from a repo audit (#58), each with a test that failed
before its fix: corrections keep metadata they do not resend, a keyed memory
moved back revives its original, duplicate text is a typed conflict, paired
tokens cannot list owner context, pairing checks the Host, sign-in at the
session cap drops the oldest session, oversized AGENTS.md no longer drops hook
context, `configure`/`setup` save the `/mcp` address, and config writes refuse
symlinks. Local checks before merge: `go test ./...`, `go vet ./...`, race
tests, 34 memory view and 31 site Node tests; CI passed on macOS, Linux and
Windows, and a Copilot review and re-review left no open findings.

The release workflow passed every job on its first run and deployed the site;
the setup page links the 2.9.7 archives. The macOS server archive matched
`SHA256SUMS` and all 53 embedded hashes, with the model and runtime
byte-identical to 2.9.6. Rehearsal on a copy of the live database: 2.9.7 passed
authenticated health, anonymous rejection, a record read at its current and
earlier revision, owner context, search and assets; the same copy reopened
under 2.9.6 with every check passing; packaged `--quickstart` started, signed
in with a session cookie and shut down cleanly.

Cutover: an encrypted off-machine backup verified first, the service stopped,
a verified snapshot was taken (271 records, 571 revisions, 361 receipts,
5 credentials), the server, backup and migrate binaries were swapped and the
service restarted. It reports 2.9.7 on loopback and over the private route and
passes the same checks with identical counts; a second encrypted off-machine
backup verified. The 2.9.6 binaries, service definition and snapshot are kept
for rollback.

Clients: on the Mac, Claude Code (client archive) and Codex (marketplace) moved
to 2.9.7. On a Windows work machine, Claude Code and Codex moved from 2.9.6 and
2.9.5 to 2.9.7 from the marketplace. The Windows PC had no connector; Claude
Code and Codex were installed from the marketplace and connected through device
approval. `grasshopper check` on all three reports server 2.9.7, verified
registration and current connectors. The Linux machine was not reachable and
updates when next used.

## 2.9.8 rollout (October 17)

2.9.8 ships memory view improvements: project pages show what agents start
with and link the startup preview (#61); agent names read as products; Archived
folds replaced handoffs; a hairline under All projects; owner-chosen device
names stored in a new `device_labels` table that never change device IDs (#62);
and a device name field that background refreshes leave alone while typing
(#63). Startup schema checks now allow 15 seconds on slow disks. Local checks
before merge: `go test ./...`, `go vet ./...`, race tests, 42 memory view and
31 site Node tests; CI passed on macOS, Linux and Windows, and each PR had a
Copilot review with findings fixed and tested (the last two #62 commits merged
under an owner-approved exception, recorded on that PR).

The release workflow passed every job on its first run and deployed the site;
`setup.js` links the 2.9.8 archives. The macOS server archive matched
`SHA256SUMS` and all 53 embedded hashes, with the model and runtime
byte-identical to 2.9.7. Rehearsal on a copy of the live database: 2.9.8 passed
every server check and renamed a device; 2.9.7 reopened that renamed copy, with
`device_labels` present, and passed every check; packaged `--quickstart`
started, signed in and shut down cleanly.

Cutover: an encrypted off-machine backup verified first, the service stopped,
a verified snapshot was taken (271 records, 571 revisions, 361 receipts,
6 credentials), the server, backup and migrate binaries were swapped and the
service restarted. It reports 2.9.8 on loopback and over the private route,
passes the same checks with identical counts and has the `device_labels`
table; a second encrypted off-machine backup verified. The 2.9.7 binaries,
service definition and snapshot are kept for rollback.

Clients: the Mac (Claude Code from the client archive, Codex from the
marketplace), the Windows PC and a Windows work machine (Claude Code and Codex
from the marketplace) moved from 2.9.7 to 2.9.8. `grasshopper check` on all
three reports server 2.9.8, verified registration and current connectors. The
Linux machine was not reachable and updates when next used.

## 2.9.9 rollout (October 17)

2.9.9 shows each opened memory as facts: Kind, Applies to, Agents (At startup,
Not until you keep it, Not at startup) and Saved by, and every row's detail
starts with its kind (#66). Archived wording says agents don't load archived
memories at startup instead of saying agents never see them. Memory-view only:
42 memory view and 31 site Node tests, `go test ./...`, CI on macOS, Linux and
Windows, and a Copilot review whose findings were fixed and tested.

The release workflow passed every job on its first run and deployed the site;
`setup.js` links the 2.9.9 archives. The macOS server archive matched
`SHA256SUMS` and all embedded hashes, with the model and runtime byte-identical
to 2.9.8. On a copy of the live database it passed every server check and
served the new memory panel.

Cutover: an encrypted off-machine backup verified first, the service stopped,
a verified snapshot was taken (271 records, 571 revisions, 361 receipts,
6 credentials), the server, backup and migrate binaries were swapped and the
service restarted. It reports 2.9.9 on loopback and over the private route with
identical counts; a second encrypted off-machine backup verified. The 2.9.8
binaries, service definition and snapshot are kept for rollback. The Mac, the
Windows PC and a Windows work machine moved their Claude Code and Codex
connectors to 2.9.9; `grasshopper check` on each reports server 2.9.9 and
current connectors. The Linux machine updates when next used.

## 2.9.10 rollout (October 17)

2.9.10 ships the audit follow-ups (#69): hooks answer unreadable input with
nothing to add and exit cleanly, the MCP bridge survives a blank or unreadable
line, connect names a full pairing queue, and scp remotes without a user and
git+ssh URLs resolve to the usual project identity. Local checks: `go test
./...`, `go vet ./...`, race tests on the client package; CI passed on macOS,
Linux and Windows, and a Copilot review left no open findings.

The release workflow passed every job on its first run and deployed the site;
`setup.js` links the 2.9.10 archives, and the clients' numeric version check
orders 2.9.10 after 2.9.9. The macOS server archive matched `SHA256SUMS` and
all 53 embedded hashes, with the model and runtime byte-identical to 2.9.9.
Rehearsal on a copy of the live database: 2.9.10 passed every server check;
2.9.9 reopened the same copy with every check passing; packaged `--quickstart`
started, signed in and shut down cleanly.

Cutover: an encrypted off-machine backup verified first, the service stopped,
a verified snapshot was taken (271 records, 571 revisions, 361 receipts,
6 credentials), the binaries were swapped and the service restarted. It reports
2.9.10 on loopback and over the private route with identical counts; a second
encrypted off-machine backup verified. The 2.9.9 binaries, service definition
and snapshot are kept for rollback.

Clients: the Mac, the Windows PC and a Windows work machine moved their Claude
Code and Codex connectors to 2.9.10, and `grasshopper check` on each reports
server 2.9.10 and current connectors. A 140 KB prompt piped into the Codex hook
returned `{}` with exit code 0 on the Mac and on the Windows PC. The Linux
machine updates when next used.

## 2.10.3 rollout (October 19)

2.10.3 lets handoffs move like any memory, shows History as full-width blocks
of earlier versions, and keeps list rows the same with or without an open
memory (#79). Local checks passed (`go test ./...`, `go vet ./...`, race tests
on memory, service and client, 72 memory-view and 31 site Node tests); PR CI
passed on all three systems and Bugbot found no issues in the handoff and
History change, with the later row-layout CSS given a final diff inspection.
In Chromium, row positions and heights matched with the panel closed and open,
and a device-only handoff moved to every device.

The release workflow passed every job and deployed the site. The macOS
archives matched `SHA256SUMS` with all 53 embedded hashes passing; packaged
`--quickstart` signed in. On a live-database copy 2.10.3 passed every check and
moved a real device-only handoff to every device with its author kept; 2.10.2
then reopened the copy. Cutover: verified off-machine backup, stop, snapshot
(274 records, 577 revisions, 366 receipts, 7 credentials), binary swap,
restart; loopback and the private route report 2.10.3 with identical counts
and a second backup verified. 2.10.2 is kept for rollback. The Mac's connectors
moved to 2.10.3, and `grasshopper check` reports server 2.10.3 and both
connectors current.

## 2.10.2 rollout (October 19)

2.10.2 fixes the memory panel layout (#77): long history no longer scrolls the
panel sideways, the panel sits against the right edge again, and handoffs say
why they have no Move to. Local checks passed (`go test ./...`, `go vet ./...`,
72 memory-view and 31 site Node tests); PR CI passed on all three systems and
Bugbot found no issues. In Chromium the panel's scroll width matched its width
at 1916, 1024 and 390 pixels. The release workflow's Windows job first failed
in the unrelated `TestWindowsHookPreservesInputAndExitStatus` (a 41-second
timeout; it passed on the same commit in PR CI); a rerun of that job passed,
and the release published and deployed the site.

The macOS archives matched `SHA256SUMS` with all 53 embedded hashes passing;
packaged `--quickstart` signed in; a live-database copy passed under 2.10.2 and
reopened under 2.10.1. Cutover: verified off-machine backup, stop, snapshot
(274 records, 577 revisions, 366 receipts, 7 credentials), binary swap,
restart; loopback and the private route report 2.10.2 with identical counts
and a second backup verified. 2.10.1 is kept for rollback. The Mac's Claude
Code and Codex connectors moved to 2.10.2, and `grasshopper check` reports
server 2.10.2 and both connectors current.

## 2.10.1 rollout (October 19)

2.10.1 lets Move to set a memory's device and OS as well as its project, so a
device-only or OS-only rule can move to another device or apply everywhere,
and insets dropdown arrows in the memory view and on the setup page (#75).
Local checks: `go test ./...`, `go vet ./...`, race tests on memory, service
and client, 71 memory-view and 31 site Node tests. CI passed on macOS, Linux
and Windows for the final head. Bugbot reviewed the device-scope change with
no findings; the following CSS-only commit had a final diff inspection. The
form and arrows were checked in Chromium, light and dark, on a synthetic
server and the setup page.

The release workflow passed every job and deployed the site; `setup.js` links
2.10.1 and the marketplace branch moved to 2.10.1. The macOS server and client
archives matched `SHA256SUMS`, all 53 embedded hashes passed, and the model
and runtime are unchanged. Packaged `--quickstart` started, signed in and shut
down cleanly. Rehearsal on a copy of the live database: 2.10.1 passed every
check and moved a real device-only memory to every device with its author
kept; 2.10.0 then reopened the same copy with every check passing.

Cutover: an encrypted off-machine backup verified first, the service stopped,
a verified snapshot was taken (273 records, 575 revisions, 365 receipts,
7 credentials), the binaries were swapped and the service restarted. It reports
2.10.1 on loopback and over the private route with identical counts; a second
encrypted off-machine backup verified. The 2.10.0 binaries, service definition
and snapshot are kept for rollback.

Clients: the Mac moved its Claude Code and Codex connectors to 2.10.1, and
`grasshopper check` reports server 2.10.1 and both connectors current. The
Windows PC, the Windows work machine and the Linux machine update when next
used.

## 2.10.0 rollout (October 19)

2.10.0 ships the memory view audit and dogfood fixes, Add memory, moves
between projects, a stable sidebar, and search relevance (#72): meaning hits
need a calibrated cutoff and must stand out from the store, and exact-word
search ignores filler words. Local checks: `go test ./...`, `go vet ./...`,
race tests on memory, service and client, 70 memory-view and 31 site Node
tests, and the real Granite tests (recall thresholds met; nonsense searches
find something in at most a third of corpora). CI passed on macOS, Linux and
Windows for the final head. Copilot reviewed four revisions; all 18 findings
were fixed with tests and the last minor round had a final diff inspection.
The memory view was dogfooded in Chromium on a synthetic loopback server at
desktop, laptop and phone widths, light and dark.

The release workflow passed every job on its first run and deployed the site;
`setup.js` links the 2.10.0 archives and the marketplace branch moved to
2.10.0. The macOS server and client archives matched `SHA256SUMS`, all 53
embedded hashes passed, and the model and runtime are byte-identical to 2.9.10.
Packaged `--quickstart` started, signed in and shut down cleanly. Rehearsal on
a copy of the live database: 2.10.0 passed sign-in, counts, current and earlier
revisions, and search ("publish" 4 results, nonsense none; 2.9.10 returned 39
for both); 2.9.10 reopened the same copy with every check passing.

Cutover: an encrypted off-machine backup verified first, the service stopped,
a verified snapshot was taken (272 records, 573 revisions, 364 receipts,
7 credentials), the binaries were swapped and the service restarted. It reports
2.10.0 on loopback and over the private route with identical counts; a second
encrypted off-machine backup verified. The 2.9.10 binaries, service definition
and snapshot are kept for rollback.

Clients: the Mac moved its Claude Code and Codex connectors to 2.10.0, and
`grasshopper check` reports server 2.10.0 and both connectors current. A live
nonsense search through the Claude Code connector returned nothing. The Windows
PC, the Windows work machine and the Linux machine update when next used.

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
