# Viewer follow-up audit results

Implemented in an isolated checkout, without dependencies, live changes, or pushes. [Acceptance scenarios](ux-audit-frontend-followup-scenarios.md) link the observable behaviors to the existing Node test harness.

| Findings | Change |
| --- | --- |
| R3-2 | Long card headings, state, metadata, omissions and device labels wrap; flexible items shrink. |
| R3-3 | External synchronous theme init precedes CSS in the viewer and all public pages; deferred theme button wiring remains separate. |
| R3-5 | Skip-to-devices link and card focus-within outline; purposeful inline review buttons remain. |
| R3-6 | Viewer banner sits outside the single main; viewer and public navigation use the Grasshopper label. |
| R4-1 | Confirm uses the server action preserving provenance; owner-edited detail names original harness/device from revision one. |
| R4-2 | Sign out everywhere and seven-day browser-session help; drafts retain their discard guard. |
| R4-3 | JSON download exposes current filters versus all records and an archived checkbox. |
| R4-4 | Kind selector scopes list/search and persists in URL state. |
| R4-5 | List-adjacent archive Undo uses acknowledged revision and a stable retry ID; expires after ten seconds or filter action. |
| R4-7 | Public SVG favicon, canonical links and sitemap included in site build; existing OG metadata retained. |
| R5-3 | Cursor-backed Show more keeps compact eight-record increments; scope/session changes reject pending pages. |
| R6-7 | Handoff card/detail text uses monospace, retaining safe text-node rendering. |

Verification: `node --test internal/gomcp/visualizer/app.test.cjs web/*.test.cjs` passes 83 tests. Running the new viewer assertions against the preceding app source demonstrates ten expected failures (67 pass). `sh web/build.sh` passes; `git diff --check` passes. Red and green logs are task-local `/tmp/grasshopper-frontend-{red,tests}.log`.

Limits: Node uses synthetic browser/API objects. Parent integration owns actual browser checks, backend behavior, theme-init asset routing, release and deployment. No real download, live session revocation, browser screenshot, or deployed result is claimed by this slice.
