# What has been verified

**Current state:** Grasshopper’s active code is Go-only. The backend and stateless client pass local tests on Mac and Work HP Windows. Go-only portable bundles passed extracted-binary checks on both machines. A live service has **not** been installed or migrated. Mac Cursor GUI now passed a fresh read and correction with the Go client. Codex Desktop and Windows Cursor GUI still need a fresh Go-client pass.

[Visual overview](how-it-works.html) · [Detailed test history](evidence-history.md) · [Setup and removal](../integrations/README.md)

## Backend and package

| Check | Observed | Limit |
|---|---|---|
| Go tests, vet, race | Passed on macOS arm64 and Work HP Windows x64. Scope isolation, confirmed preferences, stale revisions, replay, archive, legacy quarantine, auth, and five-tool MCP are covered. | Automated tests use synthetic data. |
| Real semantic recall | Pinned BGE ONNX ran on both OSes. New content was found by different wording without a restart. The small eight-query set scored 7/8 top-1 and 8/8 top-3. | Not a quality benchmark for a real profile. |
| Backup and migration | Consistent backup, copy-only re-embedding, ID/history preservation, and Go reopen after accepted writes passed on synthetic databases. | No live migration or service-manager rollback. |
| Go-only bundles | Mac arm64 and Windows x64 archives passed extraction and SHA-256 checks. Extracted servers returned 401 without auth and 200 with auth. Extracted Go clients listed five tools and loaded context; stopped-backend hooks did not claim a save. | No signed installer or persistent installation. |

## Instructions and harnesses

| Target | Observed with the Go backend | Go-client status |
|---|---|---|
| Codex Desktop, Mac 26.917.51856 and Work HP 26.917.6896.0 | Both fresh tasks received root AGENTS.md before tool use, fetched missing memory with one MCP call, and acknowledged synthetic corrections. Mac nested guidance was traced before a reversible edit. Both reported one bounded outage without a false save. | These desktop pilots used the retired bridge. Fresh Desktop Go-client check open. Windows nested read order was self-reported; interactive resume/compaction open. |
| Cursor GUI, Mac and Work HP 3.21.18 | Earlier pilots on both OSes corrected Go-backed records through the retired bridge. **Fresh Mac Go-client GUI:** root AGENTS.md marker loaded, one MCP context call returned record 1 revision 5, and one approved `store` acknowledged revision 6. Mac Codex CLI fetched revisions 6 and 5. | Fresh Work HP GUI Go-client check open. Startup hook output still did not reliably reach the first turn. The new Mac Cursor reply reported the store receipt but did not make its requested `get`; Codex independently read both revisions. |
| Claude Code 2.1.280, Mac and Work HP | Fresh Go-client hooks delivered root markers and context in tool-free turns on both OSes. Mac read Cursor revision 6 and provenance. Work HP read Mac Cursor revision 6 through a temporary loopback forward, then used its local Go client to acknowledge a synthetic correction from revision 4 to 5 and read prior content. | Mac’s first print run appended an unrelated Python-script line; later fresh reads were clean. Interactive resume/subagent checks open. |
| Codex CLI 0.155.1, Mac | With explicit Go-client MCP configuration, fresh tasks loaded root AGENTS.md, called context/get, and read Mac Cursor revisions 6 and 5. Through a temporary forward, another task read Work HP Claude revision 5 and prior revision 4 with harness provenance. | First project-local config attempt was not registered; explicit supported config override fixed it. CLI does not substitute for Desktop. |
| Cursor Agent CLI 2026.09.18, Mac and Work HP | Mac CLI saw the Go-client project MCP source as pending approval. A `--print --approve-mcps` run still rejected its context call twice. Work HP CLI previously failed to discover or approve the project MCP source while GUI worked. | Headless CLI limitation remains; GUI acceptance is separate. |

## Cross-machine behavior

With **native Go clients on both machines**, a fresh Work HP Claude Code session received Mac Cursor’s synthetic global revision 6 (`silver meadow`) in startup context. In the other direction, Work HP Claude Code acknowledged revision 5 (`amber orchard`) on its local Go backend; Mac Codex CLI fetched that revision, the prior revision 4, and Claude provenance. Both used approved temporary loopback-only Tailnet forwards. The forwards were closed after each test. Backend regression tests cover project A bun vs project B npm, device/OS separation, concurrent revision conflicts, and replay. A six-way exchange among all three desktop apps on both OSes remains open.

## Deployment and next check

No live database, public route, supervisor, or real profile was changed. No running Grasshopper deployment was confirmed on this Mac. The approved temporary Tailnet forwards and pilot listeners were stopped after cross-machine tests.

**Next check:** run Codex Desktop on Mac and Work HP plus Work HP Cursor GUI with the packaged Go client, then rehearse persistent startup and rollback on an approved private host. Do not release or migrate live data on the strength of these synthetic tests alone.
