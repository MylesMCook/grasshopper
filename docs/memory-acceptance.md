# What has been verified

**Current state:** Grasshopper’s active code is Go-only. Native backend/client tests and portable bundles passed on Mac, Work HP Windows, and Beelink Ubuntu. Fresh synthetic CLI context passed in all three harnesses on all three machines. Work HP Codex needed a Windows first-prompt fallback; its installed CLI and a task-local newer CLI both received the record before tool use. No live service has been migrated or replaced. Desktop checks are user-led if an issue appears.

[Visual overview](how-it-works.html) · [Detailed test history](evidence-history.md) · [Setup and removal](../integrations/README.md)

## 2.0.0 release checks, 24 September 2026

All records below were synthetic. The shared pilot used one Mac loopback Go
server and temporary SSH forwards to Windows and Linux. Its forwards, listener,
and remote token copies were removed after the checks.

| Gate | Direct observation | Still open |
|---|---|---|
| One shared backend | Work HP Codex CLI 0.156.1 and Claude Code 2.1.280 received global record 1, revision 2, `pebble atlas` before tools. Mac Cursor Agent 2026.09.23 fetched the same revision with one project MCP `context` call. Beelink Codex CLI 0.156.1 and Claude Code 2.1.281 received it in fresh turns; Beelink Cursor Agent fetched it with one project MCP call. Project A `bun`, project B `npm`, and a Windows path stayed scoped in direct bridge reads. | A single six-way agent correction sequence and persistent private HTTPS route have not been tested. These results do not prove Desktop behavior. |
| Windows lifecycle | Work HP Codex fresh, second prompt, and resume read the current revision; one later resume received revision 4 through hook context before tools. Claude fresh and resume read current revisions. A real Claude subagent received root guidance and revision 5; `/compact` produced a boundary and a successful `SessionStart:compact` hook response. | Codex compaction and independent Codex subagent hook were not isolated. Claude's first model turn after compact was not isolated from a resume hook. |
| Cursor CLI tool access | On Mac, plugin-only loading did not register tools across four documented/discovered manifest forms. Project `.cursor/mcp.json` plus read allowlist listed five tools and a fresh Composer 2.5 turn fetched the shared record. Beelink's fresh Composer turn did the same with one approved MCP call. [CLI setup](cursor-cli.md). | Native Git marketplace installation and IDE behavior remain untested. |
| Live memory view | Go HTTP tests cover authorization, scope, updates, and omission. Chromium on Mac showed a new confirmed record within the next three-second refresh, no 390px overflow, and a clear stale state on outage; restart showed the same records. Extracted server archives on Mac, Windows, and Linux returned 401 without auth and displayed a newly saved synthetic revision through the authenticated view API. [View guide](memory-visualizer.md). | Browser behavior on Windows/Linux and a persistent private HTTPS route remain untested. |
| Mac mini host | No persistent Grasshopper job or route was present. A task-local online backup restored revision 1 after the source advanced to revision 2; restored `get` and `context` matched, and SQLite integrity was `ok`. [Private deployment plan](mac-mini-deployment.md). | No off-host Grasshopper backup or Time Machine destination is configured. No live supervisor, private route, or real database was changed. |

**Live-view accessibility:** In Mac Chromium, keyboard focus followed the
form's reading order and had a visible outline. At 200% page zoom, the form
and records reflowed without horizontal scrolling. A custom 32px browser root
font now gives body and record text 32px; the heading still fits a 390px
viewport. Tested light/dark status, focus, and control colors met their
respective text or non-text contrast thresholds. This was a synthetic browser
check, not an all-browser accessibility certification.

**Native 2.0.0 rehearsal:** On Mac, Windows, and Linux, Go tests with the real
pinned BGE model, vet, native builds, and extracted server authentication
passed. All three authenticated visualizer APIs returned a newly saved
synthetic record. Windows needed a test-script correction to parse MCP's SSE
reply; the server acknowledged the write throughout.
The source for these OS tests was `75b417f`. Isolated Codex and Claude client
install/update/removal passed on all three OSes. Cursor's project MCP path
passed live CLI reads, but native marketplace installation remains untested.
The Mac 2.0.0 extracted client stopped-backend hook returned in 0.08 seconds with no
save claim. Its extracted server's real BGE search returned the synthetic record
by different wording with `semantic_ready=true`.

**Published package:** [v2.0.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.0.0)
contains six native client/server archives built from `45a7e08` and a release
`SHA256SUMS`. All 25 client entries per OS passed internal SHA-256 checks;
server checks passed 73 on Mac, 72 on Windows, and 71 on Linux. All packaged
local Markdown links resolved, and GitHub's published asset digests matched
the locally checked archives. The final Mac extracted server passed auth,
visualizer CSS, and a synthetic write/read. Final
[CI](https://github.com/MylesMCook/grasshopper/actions/runs/36058234153)
passed on macOS, Windows, and Ubuntu. Windows/Linux final archive changes were
documentation and visualizer CSS; their earlier native runtime tests were not
repeated on the final bytes. These are unsigned archives, not installers.

## Backend and package

| Check | Observed | Limit |
|---|---|---|
| Go tests, vet, race | Passed locally on macOS arm64, Work HP Windows x64, and Beelink Ubuntu 24.04.4 x64. [CI for package commit `38305a0`](https://github.com/MylesMCook/grasshopper/actions/runs/36042724512) passed tests, vet, and builds on macOS, Windows, and Linux; race checks ran on macOS and Linux. Scope, revisions, replay, archive, legacy quarantine, auth, and five-tool MCP are covered. | Automated tests use synthetic data. |
| Real semantic recall | Pinned BGE ONNX ran on all three OSes. New content was found by different wording without a restart. The small eight-query set scored 7/8 top-1 and 8/8 top-3 on the earlier Mac/Windows pilot. | Not a quality benchmark for a real profile. |
| Backup and migration | Consistent backup, copy-only re-embedding, ID/history preservation, and Go reopen after accepted writes passed on synthetic databases, including Beelink. | No live migration or service-manager rollback. |
| Go-only bundles | Mac arm64, Windows x64, and Linux x64 archives passed extraction and SHA-256 checks. Extracted servers returned 401 without auth and 200 with auth. Extracted Go clients listed the five memory tools and loaded context; stopped-backend hooks did not claim a save. | No signed installer or persistent installation. |

**Earlier physical Linux backend pilot:** On Beelink, task-local Go 1.27.1 and ONNX Runtime 1.30.0 passed `go test -count=1 ./...` with real BGE tests, `go vet`, race checks, and native builds. The Go client exercised all five tools against a synthetic loopback server: scoped bun/npm decisions stayed separate; writes were immediately searchable; replay, conflicts, full historical reads, and archive/restore behaved as expected. A post-write backup reopened with its revisions intact. The extracted Linux archive passed 52 checksums and read that restored backup. With the server stopped, the hook and write failed in under 0.02 seconds without claiming persistence. Agent-harness checks came later, in the package pilot below.

### Client plugin package, synthetic 0.1.6–0.1.8 archives

| OS and harness | Observed | Limit |
|---|---|---|
| Mac arm64: Codex CLI 0.156.1/Luna | Isolated local marketplace installed 0.1.3, then removed and installed 0.1.5. MCP registered; a fresh turn received global record 1 revision 4 before tool use. | Hook trust was established in the isolated test home. Current Desktop plugin behavior is not inferred. |
| Mac arm64: Claude Code 2.1.280/Haiku | Isolated marketplace install/update succeeded. A fresh 0.1.5 `--plugin-dir` turn received record 1 revision 4 before tool use. | Isolated installed copy lacked its own sign-in; model run used the signed-in CLI with a task-local plugin path. |
| Mac arm64: Cursor Agent 2026.09.23/Composer 2.5 | A fresh `--plugin-dir` turn received record 1 revision 4 from startup context. A second turn with an explicit read allowlist found no registered Grasshopper MCP server from the plugin alone. | Project-local MCP setup remains necessary for CLI tools. Native Git marketplace/IDE installation is untested. |
| Work HP Windows x64: Claude Code 2.1.280/Haiku; Cursor Agent 2026.09.23/Composer 2.5 | Claude's fresh package turn received root guidance and record 1 revision 2. Cursor's fresh turn read record 1 and Windows scope through a project-local MCP source and read allowlist. | Cursor's plugin-only MCP did not register. Work HP tests were CLI-only in a scratch folder. |
| Work HP Windows x64: Codex CLI 0.155.1 and task-local 0.156.1/Luna | The 0.1.8 package passed 24 hashes. Fresh turns on both versions received record 1 revision 2 and root marker `PACKAGE-WIN-613` before tool use. The compiled hook passed start-to-first-prompt fallback and second-prompt skip checks. With the backend stopped, Codex reported memory absent without a save claim. Cleanup removed the isolated plugin, token, auth link, and listener. | Tested CLI turns used `UserPromptSubmit`; `SessionStart` did not launch. A second interactive prompt, resume, compaction, and Desktop remain untested with 0.1.8. An earlier 0.1.7 interactive turn passed. |
| Beelink Ubuntu x64: Codex CLI 0.156.1/Luna; Claude Code 2.1.281/Haiku; Cursor Agent 2026.09.23/Composer 2.5 Fast | All three fresh turns reported synthetic record 1 revision 1 and project AGENTS marker from startup context. Claude called plugin `context`; Cursor called project-local `context` with `scope: {}`. The 0.1.6 archive verified 24 hashes and its direct hook loaded the record. | Cursor's first full Composer turn timed out at 90 seconds; Composer Fast passed in about 52 seconds. Plugin-only MCP remains unproven. No Linux desktop inference. |

The Mac and Windows synthetic archives were also verified with 24 SHA-256 entries. All package tests used temporary loopback backends and synthetic data; their listeners were stopped and temporary tokens removed. Local Codex and Claude install, update, and removal paths were exercised in isolated configurations; removal left no plugin or MCP registration there. Cursor CLI's marketplace command accepts a Git source, so local archive install/update/removal was not claimed as native distribution.

## Instructions and harnesses

| Target | Observed with the Go backend | Go-client status |
|---|---|---|
| Codex Desktop, Mac 26.917.51856 and Work HP 26.917.6896.0 | Both fresh tasks received root AGENTS.md before tool use, fetched missing memory with one MCP call, and acknowledged synthetic corrections. Mac nested guidance was traced before a reversible edit. Both reported one bounded outage without a false save. | These desktop pilots used the retired bridge. Current Go-client app behavior is untested and user-led if an issue appears. Windows nested read order was self-reported; interactive resume/compaction open. |
| Cursor GUI, Mac and Work HP 3.21.18 | Earlier pilots on both OSes corrected Go-backed records through the retired bridge. **Fresh Mac Go-client GUI:** root AGENTS.md marker loaded, one MCP context call returned record 1 revision 5, and one approved `store` acknowledged revision 6. Mac Codex CLI fetched revisions 6 and 5. | Work HP GUI with the current Go client is untested and user-led if an issue appears. Startup hook output still did not reliably reach the first turn. The new Mac Cursor reply reported the store receipt but did not make its requested `get`; Codex independently read both revisions. |
| Claude Code 2.1.280, Mac and Work HP | Fresh Go-client hooks delivered root markers and context in tool-free turns on both OSes. Mac read Cursor revision 6 and provenance. Work HP read Mac Cursor revision 6 through a temporary loopback forward, then used its local Go client to acknowledge a synthetic correction from revision 4 to 5 and read prior content. | Mac’s first print run appended an unrelated Python-script line; later fresh reads were clean. Interactive resume/subagent checks open. |
| Codex CLI 0.155.1, Mac | With explicit Go-client MCP configuration, fresh tasks loaded root AGENTS.md, called context/get, and read Mac Cursor revisions 6 and 5. Through a temporary forward, another task read Work HP Claude revision 5 and prior revision 4 with harness provenance. A separate [state-evolution probe](research-state-evolution.md) used `context` and historical `get` to explain a synthetic correction accurately. | First project-local config attempt was not registered; explicit supported config override fixed it. No current Desktop inference. |
| Cursor Agent CLI 2026.09.23-86fc751, Mac | A fresh project-local MCP source and read allowlist fetched current and prior revisions with provenance. Headless `store` was rejected; an interactive one-time approval acknowledged revision 3, and a fresh read found revisions 3 and 2. | Server and tool approval are separate. This isolated test used `--approve-mcps`; normal setup should approve only Grasshopper. Work HP and Beelink CLI reads were checked later. No Desktop inference. |
| Claude Code CLI 2.1.280, Mac | A fresh, read-only print session used the Go client to get current and prior revisions, explain “concise” → “direct,” and cite provenance. | The correction-history probe has not been repeated on Work HP. Resume/compaction remain open. |

**Mac lifecycle probe (Codex CLI 0.156.1; Claude Code CLI 2.1.280):**

- Claude Code did not load the pilot root `AGENTS.md` natively in this installed session. The Go `SessionStart` fallback delivered root marker `ALDER-612` and synthetic memory before tool use. A separate turn read nested marker `MAPLE-347` before its file. The [official agents-md mod](https://github.com/anthropics/claude-code/blob/main/mods/agents-md/README.md) describes native support; this observed setup still needed the fallback.
- Claude resume picked up an explicit correction from revision 2 to 3 without a tool call. A fresh subagent reported root guidance, the user AGENTS.md review rule, and revision 3 in its first tool-free answer. The trace could not separate its own hook from parent-context inheritance.
- Codex loaded root, user, and nested AGENTS.md guidance natively. Before hook trust, a fresh turn needed one MCP context call. After the user trusted both hooks in the new `~/Code/MylesMCook/grasshopper-native-pilot`, a fresh tool-free turn received record 1 revision 3 from the current Go `SessionStart` hook.
- A persisted Codex session saw revision 3. After an explicit synthetic correction, `codex exec resume` saw revision 4 (`slate otter`) before tool use and identified revision 3 as stale. Codex compaction and independent subagent-hook delivery remain untested.

**Fresh self-host trial (Mac):** `--create-db` created an empty memory-only SQLite database in a task-local folder. Unauthenticated health returned 401 and authenticated health 200. An MCP `store` acknowledged synthetic record 1 revision 1, `get` returned its full content, and semantic search found it by different wording without a restart (`semantic_ready=true`). Restarting with `--create-db` kept the record. The Go unit test also checks that direct initialization refuses to overwrite an existing database and that a consistent copy reopens. A new macOS archive verified all 66 checksums, contained the visual guide and its fonts, and its extracted server started a second fresh database with 401/200 health behavior. No live database was touched.

**Visual guide:** The [HTML overview](how-it-works.html) uses the requested [Myles Design Foundation](https://github.com/MylesMCook/myles-design-md/blob/main/DESIGN.md). A real browser loaded both bundled fonts, found the expected sections, and had no horizontal overflow at 390px. It remains a static explanation, not a service UI.

## Cross-machine behavior

With **native Go clients on both machines**, a fresh Work HP Claude Code session received Mac Cursor’s synthetic global revision 6 (`silver meadow`) in startup context. In the other direction, Work HP Claude Code acknowledged revision 5 (`amber orchard`) on its local Go backend; Mac Codex CLI fetched that revision, the prior revision 4, and Claude provenance. A fresh Mac Codex CLI turn also received Beelink's synthetic global record 1 revision 1 (`amber heron`) from its trusted Go startup hook over a temporary loopback-only Tailnet forward, without a tool call. After the Beelink server stopped, another turn reported unavailable memory and no saved write. The hook returned that failure in about 0.03 seconds. All forwards and test listeners were closed. Backend regression tests cover project A bun vs project B npm, device/OS separation, concurrent revision conflicts, and replay. A six-way exchange among all three CLIs on Mac and Windows remains open.

## Deployment and next check

No live database, public route, or real profile was changed. No running Grasshopper deployment was confirmed on this Mac. Beelink's older services were retired by a separate host-maintenance task before this fresh pilot; their data and configuration were preserved. This task ran a new synthetic Go server on Beelink loopback port 18106, confirmed unauthenticated 401 and authenticated 200, listed the five tools, saved and read one synthetic record, then stopped the server and removed its temporary token. The Mac loopback forward and pilot listeners were closed.

**Codex hook trust:** The older pilot's trusted hook delivered revision 3 before tool use. The new Code-location pilot initially had no trust and delivered no startup memory. After the user reviewed and trusted its exact `SessionStart` and `SubagentStart` hook hashes, a fresh Codex CLI turn received revision 3 through the current Go client before tool use. The Beelink read above used the same trusted hook with a temporary client URL; the original client config was restored afterward.

**Remaining checks:** Cursor's native Git marketplace installation and Codex compaction/subagent delivery are open. Work HP testing is CLI-only and must avoid the user's real work. Beelink remains shared with a separate host-maintenance task. The user will test desktop apps if an issue appears. A software release does not authorize a live database migration or private-route deployment; those require separate approval and recovery checks.
