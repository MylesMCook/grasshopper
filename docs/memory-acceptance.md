# What has been verified

**Current state:** [Grasshopper 2.0.2](https://github.com/MylesMCook/grasshopper/releases/tag/v2.0.2) is published. The private Mac mini service has a newer visualizer and a narrowly approved Codex annotation CSP allowance; this fix is not in the 2.0.2 archives. Encrypted off-host backup and restore passed. Fresh Codex and Cursor CLI turns on Mac mini and Beelink read the same live project memory. Work HP Codex and Cursor model turns remain unverified. No legacy data was migrated or personal profile seeded.

[Visual overview](how-it-works.html) · [Detailed test history](evidence-history.md) · [Setup and removal](../integrations/README.md)

## Prepared site, browser session, and marketplace

This source build adds a plain-language `/about/` page with a two-way link to the memory view, a shared light/dark setting, and a signed, HttpOnly visualizer cookie that survives a reload. A task-local synthetic server and Chromium checked connect, reload, disconnect, no bearer value in browser storage, both theme directions, the shared icon switch, footer privacy link, sticky header, and no overflow at 390px. The source-file preview points its view links at the default loopback port; browser automation could not open `file://` because of its URL policy, so that fallback has not been clicked in an automated browser. Hosted-page links were clicked both ways. Go tests cover cookie expiry, tampering, token rotation, origin checks, and MCP/health denial. Grasshopper's own repo marketplace package passed portable and OpenAI manifest validation. This build has **not** been deployed or fresh-installed from the Git marketplace; the live Mac mini still runs the earlier annotation fix.

**2.1.0 draft gate:** [CI on `fc2ee15`](https://github.com/MylesMCook/grasshopper/actions/runs/36085779368) passed Mac, Linux, and Windows tests, vet, and native builds; Mac and Linux race checks passed. An earlier Linux race run caught a cookie-signature test weakness, corrected in `fb7360b`; the final run passed. All six draft archives passed their internal SHA-256 checks and GitHub asset digests match the external checksum file. The extracted Mac server passed synthetic authentication, website, remembered-session, disconnect, and cookie-versus-MCP checks. Linux and Windows archives contain native binaries and passed CI, but their 2.1.0 packages were not run on physical hosts. A Beelink SSH attempt stopped at host-key verification; no host key was bypassed. The draft is not published and the live service is unchanged.

## Visualizer selection check, 24 September

**Public-site annotation:** The live Cloudflare site initially returned `style-src 'self'`, and Codex Annotate could not select text. A header-only Wrangler deployment left that policy at the edge. After the build tied page asset hashes to `_headers`, the deployed home, `/view/`, and 404 responses carried the two exact style hashes proven on the private memory view. The user refreshed the home and `/view/` pages and selected each heading in Codex Annotate. The private `/about/` page has the same opt-in policy in source but has not been deployed.

The previous page rebuilt its record DOM on every three-second poll, clearing selected text. A task-local Mac server with an empty synthetic database reproduced the polling path. After the fix, Chromium kept the same DOM node and the same selected text through a live refresh. A synthetic record remained selected when the next response changed, then updated after selection ended. Its Details disclosure opened, a 390px viewport had no horizontal overflow, and the browser console had no errors. `go test ./...`, `go vet ./...`, JavaScript syntax, and `git diff --check` passed. Those browser checks alone did not prove Codex's in-app comment affordance; the live checks above and below did. No live memory was written because this task does not expose the Grasshopper MCP plugin.

**Codex annotation finding:** The user tried the updated private page; annotation still could not select an element. In ChatGPT/Codex app build 26.917.71314, the in-app browser logged two inline-style CSP violations against `style-src 'self'`. The injected annotation stylesheet had no CSSOM sheet, and its interaction layer measured 12.5px by 12.5px instead of covering the viewport. A task-local server allowed only the exact two hashes from this build, and the user selected the heading in Codex annotation mode. After explicit approval, those hashes were enabled only on the live visualizer document; the user then confirmed that live annotation selection worked. Local and private health/API returned 200 with auth, anonymous health/API returned 401, SQLite integrity was `ok`, and record/revision counts stayed at seven/fourteen.

## 2.0.2 release check

[CI](https://github.com/MylesMCook/grasshopper/actions/runs/36079552814) passed tests, vet, native builds, and the configured race checks on the release commit `d3212c9` across Mac, Windows, and Linux. The six client/server archives passed every internal SHA-256 entry; GitHub's seven asset digests match the six archives and external `SHA256SUMS` file. All three client manifests identify version 2.0.2. The Mac and Linux server archives were extracted and started with synthetic databases: authenticated health and visualizer API reads returned 200, anonymous reads returned 401, and the public static view shell loaded. Beelink's temporary smoke token was removed and its test server stopped. Windows 2.0.2 binaries were built and tested in CI and verified as PE x64 files; the Windows archive was not run on a physical host. No live database or service was changed for packaging.

## Same Git project: fresh CLI turns on two machines

Task-local clones had different paths and the same normalized Git `origin`: `git:github.com/MylesMCook/grasshopper`. A synthetic project-only handoff, record 7 revision 1, was written to the live private service. It was not a user preference.

| Client | Actual result |
|---|---|
| Mac mini Codex CLI 0.156.1 | Fresh turn called MCP `context` once and read record 7, revision 1, including an unseen marker. |
| Beelink Codex CLI 0.156.1 | Fresh turn independently called MCP `context` and read the same record and revision. |
| Mac mini Cursor Agent CLI 2026.09.23-86fc751, Composer 2.5 | Fresh `--print --auto-review` turn called MCP `context` once and read the same record. A second turn passed using the packaged exact read-tool allowlist. |
| Beelink Cursor Agent CLI 2026.09.23-86fc751, Composer 2.5 | Fresh `--print --auto-review` turn called MCP `context` once and read the same record. |

Default headless Cursor `--print` rejected the MCP read on both machines even after server approval; `--auto-review` succeeded. Beelink's forced CLI sandbox stopped at local AppArmor, before a model turn. No host policy was changed. All four successful turns were CLI tests, not desktop-app tests. The synthetic record was archived at revision 2; active context omitted it and historical `get` retained revision 1. Private receipts are in `~/Documents/Codex/2026-09-24-grasshopper-cross-device-project/cli`.

## 24 September follow-up: recovery and memory view

| Gate | Observed | Open |
|---|---|---|
| Recovery | Fresh encrypted R2 backup restored byte-for-byte. SQLite integrity returned `ok`. A separate server read record 1 at revisions 2 and 1 from the copy. The live writer stayed running. | Reboot, Mac-loss recovery, and Bitwarden sync on another device. |
| Memory view | A failing regression test established the old empty device view. Synthetic HTTP tests cover active project/device choices, project isolation, and agent-context separation. Mac Chromium showed Mac and Windows facts, project A without B, working manual entry, no 390px overflow, empty browser storage, and clean Disconnect. The updated view shell and authenticated API now respond through the private route. | The live database has no active records to display; live-route browser interaction was not repeated. |
| Main connectors | Fresh Mac mini and Beelink Codex/Cursor CLI turns each read the same live project record. Work HP hooks reached Grasshopper; its Codex/Cursor model turns timed out, without isolating a service defect. | Work HP is deferred. Cursor native marketplace installation remains untested. |
| Lifecycle | Fresh and resumed Codex sessions received updated context. | Codex compaction and independent child `SubagentStart` delivery have not been isolated. Cursor native Git marketplace installation is untested. |

Checks for the new view: `go test -count=1 ./...` with the pinned model, `go vet ./...`, race tests for memory/MCP/client, JavaScript syntax, and `git diff --check` all passed. The test server and browser sessions were stopped.

**Mac mini update:** The new binary identified clean commit `5e19b96`. Before restart, the database passed SQLite integrity and the old binary/plist were copied to a private task-local rollback directory. Only `com.myles.grasshopper` was restarted. Afterward, it listened on `127.0.0.1:8106`; local and Tailnet health were `200` with the token and `401` without it. Anonymous visualizer API and MCP requests returned `401`; the static view shell contains no memory data. The private view API returned empty active records and project/device lists, as expected after synthetic pilot cleanup. Private MCP `get` read record 1 at revisions 2 and 1. The R2 backup job retained its daily schedule and last exit code 0. Tailscale Serve kept the existing Tailnet-only `8456 → 8106` mapping. No data, client config, or other service was changed.

**Same repo on Mac mini and Beelink (24 September):** The Mac clone (`9b42774`) and older Beelink clone (`d306eb7`) had the same HTTPS `origin`, different paths, and no project-ID override. Both 2.0.1 hooks independently resolved `git:github.com/MylesMCook/grasshopper` and reached the live Mac service.

- Mac saved synthetic project-only handoff 5. Beelink's `context` returned revision 1.
- Beelink saved handoff 6. Mac's `context` returned revision 1. An unrelated project scope returned neither.
- The opposite machine archived each record at revision 2. Active context omitted them; `get` retained revisions 1 and 2. The temporary Beelink credential and config were removed.

This proves the physical two-machine hook and MCP path, not a fresh Codex or Cursor model turn. Git aliases, repo renames, and local ID overrides remain untested. Private receipts are in `~/Documents/Codex/2026-09-24-grasshopper-cross-device-project`.

## 2.0.1 release check

Six Mac, Windows, and Linux client/server archives were packaged at `dd15c15`; all internal SHA-256 entries and GitHub asset digests match. [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36067286155) passed on that commit. Native tests with the pinned model, vet, race checks, and extracted-server authentication/proxy checks passed on all three systems. Windows and Linux reused binaries tested at code-identical `72a460c`; the intervening commit changed only three documents. The final Mac archive was installed into the private launchd job; authenticated local health, live MCP context, and the scoped visualizer API passed. The private route remains Tailnet-only.

Fresh Mac Codex CLI 0.156.1 loaded root AGENTS.md, resolved the stable project ID with a read-only Git check, and called Grasshopper `context` once to read project record 4 revision 1 from the running service. Mac Claude Code 2.1.280 and Cursor Agent 2026.09.23 received the same record from startup context before tool use. On Beelink, fresh Codex CLI 0.156.1 and Cursor Agent 2026.09.23 used the corrected `id:` scope through MCP against a task-local server built from the release source; Claude Code 2.1.281 read the live Mac service earlier. Work HP Claude Code 2.1.280 read the live service in its first turn. Work HP Codex and Cursor direct hooks loaded the record, but bounded model turns did not finish; Cursor's project MCP still needed approval. These are CLI results, not Desktop results.

The final Mac client package passed isolated Codex and Claude install, 2.0.0→2.0.1 upgrade, and removal checks. Cursor's project MCP configuration listed all five tools against a synthetic server; native marketplace installation remains untested. After the CLI checks, the live synthetic record was archived at revision 2. An encrypted off-host snapshot restored byte-for-byte, passed SQLite integrity, and retained its archived state and revision 1 history. No personal profile was seeded.

## Private-service check, 24 September 2026

| Gate | Observed | Still open |
|---|---|---|
| Mac mini service | Fresh database on loopback `8106`, one launchd writer, Tailnet-only Serve HTTPS `8456`. Health returned `401` without a token and `200` with one. Existing Serve mappings were unchanged. Two synthetic scoped records were written and archived; no profile or legacy data was imported. | Reboot persistence and full real-profile use. |
| Off-host recovery | Dedicated R2 bucket and bucket-scoped token; restic encrypted upload passed a full-data check. A restore matched the source bytes and passed SQLite integrity. After a synthetic write, a later restored server read current and prior revisions. Daily backup launchd job passed an explicit run. Recovery values were saved to a Bitwarden secure note without entering Git. | Bitwarden vault sync from another device, Mac-loss recovery, and retention policy. |
| Second device | Beelink reached private HTTPS. Unauthenticated health/MCP returned `401`; authenticated MCP `context`, `store`, `get`, and `archive` succeeded against the same Mac database. | Live CLI turns from Codex, Cursor, and Claude on all intended machines. Work HP remains CLI-only and non-disruptive. |
| Private proxy | The published 2.0.0 server returned `403` on Beelink MCP POST because the SDK rejected the private HTTPS Host. A regression test reproduced it. The installed 2.0.1 archive accepts one configured exact proxy Host, rejects others and mismatched Origins, and kept local MCP working. Beelink repeated authenticated `context`, `store`, `get`, and `archive` on the earlier checked candidate; final extracted archives passed proxy smoke tests on all three systems. | Full CLI compatibility is tracked above and below. |

The first cross-machine proxy check used the 2.0.1 candidate archive at `0d4bb53`; the checked release binary now runs on the Mac. Detailed host setup and rollback are in [Mac mini private service](mac-mini-deployment.md). These checks do not establish public accessibility; the route uses Tailscale Serve, not Funnel.

**Earlier CLI finding:** Mac Codex CLI 0.156.1 initially sent a folder path, then `Darwin`, and missed context. Corrected shared policy and MCP schema fixed its task-local shadow-server run and the final live retry above. Work HP Codex CLI 0.155.1 stalled past its 120-second model-turn cap. Cursor Agent CLI 2026.09.23 timed out at 90 seconds; its project MCP source still needed approval. Both direct startup hooks loaded the record, but neither model turn proved receipt. Cursor's headless `--mode ask` denied MCP; normal `--print` with a project read allowlist and explicit MCP approval worked on Mac and Beelink. These observations are CLI evidence, not Desktop proof.

## 2.0.0 release checks, 24 September 2026 (historical)

All records below were synthetic. The shared pilot used one Mac loopback Go
server and temporary SSH forwards to Windows and Linux. Its forwards, listener,
and remote token copies were removed after the checks.

| Gate | Direct observation | Still open |
|---|---|---|
| One shared backend | Work HP Codex CLI 0.156.1 and Claude Code 2.1.280 received global record 1, revision 2, `pebble atlas` before tools. Mac Cursor Agent 2026.09.23 fetched the same revision with one project MCP `context` call. Beelink Codex CLI 0.156.1 and Claude Code 2.1.281 received it in fresh turns; Beelink Cursor Agent fetched it with one project MCP call. Project A `bun`, project B `npm`, and a Windows path stayed scoped in direct bridge reads. | A single six-way agent correction sequence was not tested in this pilot. These results do not prove Desktop behavior. |
| Windows lifecycle | Work HP Codex fresh, second prompt, and resume read the current revision; one later resume received revision 4 through hook context before tools. Claude fresh and resume read current revisions. A real Claude subagent received root guidance and revision 5; `/compact` produced a boundary and a successful `SessionStart:compact` hook response. | Codex compaction and independent Codex subagent hook were not isolated. Claude's first model turn after compact was not isolated from a resume hook. |
| Cursor CLI tool access | On Mac, plugin-only loading did not register tools across four documented/discovered manifest forms. Project `.cursor/mcp.json` plus read allowlist listed five tools and a fresh Composer 2.5 turn fetched the shared record. Beelink's fresh Composer turn did the same with one approved MCP call. [CLI setup](cursor-cli.md). | Native Git marketplace installation and IDE behavior remain untested. |
| Live memory view | HTTP tests cover authorization, scope, updates, and omission. Chromium on Mac showed a new confirmed record within the next three-second refresh, no 390px overflow, and a clear stale state on outage; restart showed the same records. Extracted server archives on Mac, Windows, and Linux returned 401 without auth and displayed a newly saved synthetic revision through the authenticated view API. [View guide](memory-visualizer.md). | Browser behavior on Windows/Linux and the persistent private route were not tested in this pilot. |
| Mac mini host | Before the persistent deployment, a task-local online backup restored revision 1 after the source advanced to revision 2; restored `get` and `context` matched, and SQLite integrity was `ok`. | This earlier trial did not test off-host backup or a private route; see the newer private-service check above. |

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

## Earlier deployment trial and remaining checks

Before the Mac mini deployment, no running Grasshopper service was confirmed there. Beelink's older services were retired by a separate host-maintenance task before this pilot; their data and configuration were preserved. This task ran a synthetic server on Beelink loopback port 18106, confirmed unauthenticated 401 and authenticated 200, listed the five tools, saved and read one synthetic record, then stopped the server and removed its temporary token. The Mac loopback forward and pilot listeners were closed.

**Codex hook trust:** The older pilot's trusted hook delivered revision 3 before tool use. The new Code-location pilot initially had no trust and delivered no startup memory. After the user reviewed and trusted its exact `SessionStart` and `SubagentStart` hook hashes, a fresh Codex CLI turn received revision 3 through the current Go client before tool use. The Beelink read above used the same trusted hook with a temporary client URL; the original client config was restored afterward.

**Remaining checks:** Cursor's native Git marketplace installation and Codex compaction/subagent delivery are open. Work HP testing is CLI-only and must avoid the user's real work. Beelink remains shared with a separate host-maintenance task. The user will test desktop apps if an issue appears. The private route is now approved and running; a legacy-data migration is still separate and unapproved.
