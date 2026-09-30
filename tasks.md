# Grasshopper release state

## Active report 11: scannability

[LAB-236](https://linear.app/mcook/issue/LAB-236): Codex on macOS owns
`codex/ui-report11` in an isolated managed worktree, based on `405a5a3`.
Separate `gpt-6.1-sol` workers own setup, home and compact memory-list slices.
Parent owns integration, actual invented-data screenshot crops, acceptance,
independent review and delivery. Proposed patch 2.8.5; live remains 2.8.4.
[Accepted outcomes and examples](docs/ui-audit-report11-results.md).
No dependencies, framework, API, authentication or data changes. Preserve
pagination, global-memory identity and search/startup ordering. Primary checkout
and unrelated private evidence remain preserved. Implementation is complete; 132 Node tests, Go/vet/race and synthetic browser
checks pass. Actual desktop/phone crops are bundled. One independent ready-PR
review, final-head CI, recovery, packaging and rollout remain before closeout.

## Released final UI audit: 2.8.4

[LAB-235](https://linear.app/mcook/issue/LAB-235): [2.8.4](https://github.com/MylesMCook/grasshopper/releases/tag/v2.8.4)
ships F1–F11 and reconciled review policy via PR #26/code `765f677`, marketplace
`35bf501`, public deployment `5f4c74bf`. Isolated Sol implementation, independent
review and targeted re-review completed before merge. All 121 Node tests,
Go tests/vet/race, native three-platform CI, seven archives/287 embedded hashes,
eight published digests, packaged quickstart, restored-copy upgrade and 2.8.3
rollback pass. Mac service and three installed Mac connectors run 2.8.4;
live owner/session/data/assets and public browser checks pass. Pre/post encrypted
off-host backup checks pass. [Delivery evidence and limits](docs/memory-acceptance.md#284-final-ui-release-and-rollout-september-30).
Documentation-only delivery receipt: PR #27 on `codex/ui-final-receipt` in an
isolated managed worktree; the PR records review and final CI. Primary checkout, prior
packages, recovery files and unrelated evidence remain preserved. Existing chats
may retain older bridges until reopened.

## Released UI audit: report 9, 2.8.3

[LAB-234](https://linear.app/mcook/issue/LAB-234): [2.8.3](https://github.com/MylesMCook/grasshopper/releases/tag/v2.8.3)
ships all report 9 outcomes via PR #24, release `335b8e3`, marketplace `96106e5`.
Codex coordinated isolated `gpt-6.1-sol` implementation, review, packaging and
live-site workers. The owner approved the completed Sol reviews and passing
final-head CI as the scoped external-review exception for PRs #23/#24.
117 Node tests, Go tests/vet/race, native three-platform CI, seven archives/287
embedded hashes, eight published digests, quickstart, restored-copy upgrade and
2.8.2 rollback pass. The Mac service, installed Mac connectors and public site
run 2.8.3. Sessions/data/credentials/routing survived; pre/post encrypted backups
pass. [Delivery evidence and limits](docs/memory-acceptance.md#283-report-9-release-and-rollout-september-30).
Documentation receipt is included in the final UI audit batch; product delivery is complete.
Primary checkout and unrelated evidence remain preserved. Existing chats may
retain previous bridges until reopened.

## Released UI audit: report 8, 2.8.2

[LAB-233](https://linear.app/mcook/issue/LAB-233): [2.8.2](https://github.com/MylesMCook/grasshopper/releases/tag/v2.8.2)
ships U1–U15 and S1–S4 via PR #22, release `df877ec`, marketplace `5ab9998`.
Codex coordinated isolated `gpt-6.1-sol` implementation, review, packaging and
live-site workers. Final-head CI and Copilot review preceded merge; the review's
metadata reflow finding was fixed and both conversations resolved. All 107 Node
tests, local Go checks, native three-platform CI, seven archives/287 embedded
hashes and eight published digests pass. Restored-copy upgrade and 2.8.1 rollback,
session continuity and pre/post encrypted off-host backups pass. Mac service,
installed Mac connectors and the public site run 2.8.2. Data, credentials and
routing were preserved. [Delivery evidence and limits](docs/memory-acceptance.md#282-report-8-ui-release-and-rollout-september-30).

Primary checkout and unrelated local evidence remain preserved. Existing chats
may retain an earlier bridge until reopened.


## Released report 7: 2.8.1

[LAB-232](https://linear.app/mcook/issue/LAB-232): [2.8.1](https://github.com/MylesMCook/grasshopper/releases/tag/v2.8.1)
ships all ten findings via PR #20, release `7f9d98f`, marketplace `1356f3e`.
Codex coordinated isolated `gpt-6.1-sol` workers; final-head CI and Copilot review
preceded merge. Local Go tests/vet/race, 103 Node tests, native three-platform CI,
seven archives/287 embedded hashes and eight published digests pass. Packaged
quickstart and restored-copy upgrade/2.8.0 rollback pass. The Mac service and
installed Codex/Cursor/Claude clients run 2.8.1; the public site is deployed and
verified. Data, credentials, sessions and routing survived; pre/post encrypted
off-host backups pass. [Delivery evidence and limits](docs/memory-acceptance.md#281-report-7-release-and-rollout-september-29).

Primary checkout and unrelated local evidence remain preserved. Existing chats
may retain their previous bridge until reopened.

## Released UX audits: 2.8.0

[LAB-231](https://linear.app/mcook/issue/LAB-231): [2.8.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.8.0)
ships PRs #16–#17, release commit `b772736` (PR #18), marketplace `2159643`.
Codex owned delivery in an isolated macOS worktree with `gpt-6.1-sol` packaging,
recovery and site reviewers. Final-head CI and Copilot review preceded the
release-preparation merge. Three-platform native CI, seven archives/287 embedded
hashes, eight published digests, real-model restored-copy upgrade and 2.7.0
recovery checks pass. The private service and installed Mac connectors run 2.8.0;
the public site is deployed and verified. Cutover preserved all 27 memory rows,
65 revisions/receipts and four device-token rows, credentials, supervisor and
routing. Readiness took 3.12 seconds. Pre/post encrypted off-host backups pass.

Live loopback/private HTTPS owner-session, pagination, conditional-response,
startup parity, authorization and exact asset checks pass. Installed Mac
Codex/Cursor/Claude startup adapters and bridges pass; a fresh isolated Claude
marketplace install passes. Public mobile/desktop browser checks pass. Eight
requested stale remote branches were deleted only after merged-head preservation
checks. [Detailed evidence and limits](docs/memory-acceptance.md#280-ux-audits-release-and-rollout-september-29).
Existing browser sessions require sign-in again; later downgrade to 2.7.0 does
not preserve session revocation. Physical devices, screen readers, persistent
Windows/Linux installations, reboot and fresh native AI turns remain untested.

## PR #16 post-merge review follow-up

[LAB-230](https://linear.app/mcook/issue/LAB-230): Codex owns an isolated macOS
worktree on `codex/post-merge-review`, based on `4af7ba1`. A `gpt-6.1-sol`
worker owns the reader regression fix in a separate worktree. Scope: one-snapshot
startup preview classification and generic public evidence references.

Accepted outcome: given active memories in scope, when startup preview loads,
each belongs to exactly one loaded or omitted category from the same snapshot;
loaded count plus exact omitted total equals the active count even when omitted
details are capped. Preserve ordering, budgets, scope and archive exclusions.

Implemented: one scoped read followed by an in-memory partition; native tests
cover complete classification, exact totals above the detail cap, scope/archive
exclusions and agreement with agent ordering. Memory/MCP tests, vet and race
checks pass. Independent `gpt-6.1-sol` review found no concrete defect. The
fixtures also pass the prior code; they verify invariants, not race reproduction.
The requested personal-path sweep is clean and branch/commit provenance remains.

Delivery gate: one PR, replies/resolution on all four old review threads, then
CI and Copilot review of the final head before merge. The LAB-230 record carries
the final PR, review and merge evidence.
Release, deployment and stale branch cleanup remain outside this follow-up.

## UX audits 1–6: merged and verified

Codex · macOS · an isolated Codex worktree on branch `codex/ux-audit-integration`; delivery receipt follows merged
`origin/main` in the same isolated checkout. Integrated released 2.7.0 evidence
`2031446` and all audit worker slices through `9d681aa`. Primary checkout and live service were
not edited. Explicit `gpt-6.1-sol` workers owned owner data, hosting, frontend and
technical review in isolated checkouts; primary owned integration and browser
verification. All workers have finished.

[Reports 1–6 and evidence](docs/ux-audit-integration-results.md) map the accepted
findings to implementation, checks and limitations. Tracking: LAB-224 through
LAB-229. The canonical global AGENTS.md now accepts concrete requested audit
outcomes without a separate scenario-approval loop while preserving protected
external, host, data and security boundaries. A private rollback copy is retained.

Final checks passed: full Go suite with pinned Granite model/runtime assets,
`go vet ./...`, race tests for client/server/backup/memory/MCP/client packages,
and all 90 Node tests. Synthetic Chromium 375×812/1280×960 checks prove first
preview at 743px, pinned maximum-text dialog,103 unique paginated records, dirty
protection, URL reload, attribution, archive Undo, revision restoration, complete
JSON download, sign-out everywhere and automatic failure recovery. Local public
build passes wrapping, selected-OS clipboard, metadata/favicon/theme/screenshot.
Review defects were fixed and retested. No new dependencies/framework.

[PR #16](https://github.com/MylesMCook/grasshopper/pull/16) merged as `6321229`.
macOS, Ubuntu and Windows CI all passed final PR head `9922260`. Its merged tree
was verified identical to that tested head. Implementation and merge are complete.
Next possible action is a separately authorized release and deployment. Release/public-site deployment/live-host changes are
separate and have not occurred. Physical devices, screen readers, native
Windows/Linux, Tailscale Serve/reboot and fresh deployed agent turns remain
untested. Task-local evidence is outside Git or under untracked `output/`.

Release owner: Codex · macOS · primary checkout on `main`.

## Released memory review and Claude marketplace

[LAB-223](https://linear.app/mcook/issue/LAB-223/release-and-deploy-the-merged-memory-review-and-claude-marketplace): [2.7.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.7.0) ships LAB-222/PRs #9–#15, code `4fea645`, marketplace `d272aa3`. Three-OS real-model CI, local tests/vet/race, 36 Node tests, seven archives/311 embedded hashes, eight published digests, synthetic browser owner workflows and restored-copy/2.6.0 rollback rehearsals pass. The Mac service runs 2.7.0 and preserves 24 chunks, 62 revisions/receipts, four token rows, credentials, model, plist and routing. Live loopback/private HTTPS reads, semantic search, device choices, exact UI assets, all-projects listing, startup-preview equality and owner permission boundaries pass. Installed Mac Codex/Cursor/Claude 2.7.0 adapters/bridges pass; the published Claude marketplace also installs in an isolated configuration. Public setup is deployed and exact bytes verified. Off-host backup and repository check pass; saved binaries/plist/snapshot retain the rollback path. [Evidence](docs/memory-acceptance.md#270-memory-review-and-claude-marketplace-september-29). No release or deployment work remains. Other machines, fresh native AI turns, desktop GUI refresh, reboot persistence and full machine-loss recovery were not retested. Existing chats can retain loaded bridges; new sessions pick up 2.7.0.

## Released connection reliability

[LAB-221](https://linear.app/mcook/issue/LAB-221/make-device-connection-identity-and-recovery-deterministic): 2.6.0 is released/deployed, code `f16d921`, marketplace `68dc3ed`. Owner-approved [scenarios](docs/connection-flow.md) implement truthful host identity, read-only approval checks, one Windows profile default and explicit verified legacy migration. Local tests/vet/race, actual native Work HP synthetic cases, three-OS real-model CI, seven package/289 embedded hashes, eight published digests and restored-copy/rollback rehearsals pass. Host cutover preserves database counts, credentials, plist, model and routing. Mac Codex/Cursor/Claude and Work HP installed Codex/Cursor 2.6.0 bridge/startup-adapter probes pass. Laptop shared/default config now reports HPLT2MQ5360JD8 using the same approved credential; legacy files remain. Codex plugin upgrade emitted its existing cache-backup warning, but installed/enabled metadata and live 2.6.0 executable checks pass. No forced interruption or ACL change occurred. Public guidance is deployed. [Evidence](docs/memory-acceptance.md#260-connection-identity-and-recovery-september-29).

LAB-221 and LAB-219 are Done after the owner requested fresh native Windows Codex verification over SSH: Codex CLI 0.155.1 loaded HPLT2MQ5360JD8 at startup and successfully called Grasshopper get for #8 revision 1. The probe used invocation-only gpt-5.5 because the CLI API rejected the configured desktop alias. The AI's read-only shell runner could not launch; actual installed connector identity/host checks passed separately. No saved model/security settings changed. Desktop GUI refresh and reboot persistence were not tested. Existing chats may retain an old bridge; reopening picks up the upgraded connector. All code, release, deployment and requested native verification work is complete; rollback and legacy files remain.

## Released memory-view simplification

[LAB-220](https://linear.app/mcook/issue/LAB-220/simplify-the-memory-view-layout-and-filter-flow): [2.5.2](https://github.com/MylesMCook/grasshopper/releases/tag/v2.5.2) is deployed on the private Mac service. Search and visible filters share one surface; exact scope precedes results; device management is secondary; structural divider lines are removed. Owner-supplied [DESIGN.md](DESIGN.md) governs bundled Geist Mono controls/body, Newsreader headings/reading, colors and 2px corners. Release commits `96fd7ff` and `6fb359e`; marketplace `46b660b`. Three-OS real-model CI, Go tests/vet/race, 10 Node tests, seven package/289 embedded hashes and eight published digests pass. Synthetic desktop/mobile checks covered search, history, manual scope, keyboard focus, contrast and 320–1280px reflow. Restored-copy and old-binary rollback rehearsals pass. Live loopback/private-HTTPS reads, semantic search, exact UI assets and laptop device choices pass. The cutover preserved 23 records, 59 revisions/receipts, four token rows, credentials, plist and private routing; rollback binaries/snapshot remain. Existing Mac Codex/Cursor/Claude 2.5.0 adapters still read through server 2.5.2 without configuration changes. [Evidence](docs/memory-acceptance.md#252-memory-view-layout-september-29).

## Laptop device mismatch resolved

[LAB-219](https://linear.app/mcook/issue/LAB-219/show-connected-devices-accurately-in-the-memory-view): approved dropdown fix `ebd3cea` is shipped. Active paired IDs appear without scoped memories; saved-memory choices remain after revocation; agent retrieval is unchanged. HPLT2MQ5360JD8 is active and present in deployed owner browse/search choices.

Approved native Windows repair replaced the Store-app redirected owner credential with a dedicated paired credential. Memory #8 revision 1, installed Windows client check and owner-controls denial passed. The owner-requested fresh native Windows Codex turn over SSH now loads the paired laptop device and reads #8 revision 1. LAB-219 is Done. Existing chats can retain old loaded credentials; the desktop GUI was not separately exercised. The prevention work is released in 2.6.0 under LAB-221. No further laptop changes are pending.

## Released memory-view improvement

[LAB-218](https://linear.app/mcook/issue/LAB-218/search-and-inspect-saved-memories-in-the-memory-view): [2.5.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.5.0) ships owner-only scoped search and full inspection of saved memories, including omitted large records, confirmation/date/source, and revision history. Release commit `2bd4927`; marketplace `160f630`. Granite, ranking, startup selection, stored records, credentials, and the five MCP tools remain unchanged. Paired Thermos review, three-OS real-model CI, package digests, synthetic desktop/mobile browser checks with real Granite semantic search, Mac rollback and live deployment, authenticated local/Tailnet reads, off-host backup, and direct Mac Codex/Cursor/Claude connector probes passed. [Evidence](docs/memory-acceptance.md#250-memory-view-search-and-inspection-september-29). No delivery work remains for this slice. Later product candidates are correction/archive controls, startup-budget allocation, and ranking trials; they have not been implemented. Research: private task-local `PRODUCT-REVIEW.md`.

## Released model upgrade

[LAB-217](https://linear.app/mcook/issue/LAB-217/improve-memory-recall-with-a-verified-model-upgrade-and-reversible): [2.4.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.4.0) ships Granite small English R2 with pinned graph/weights/tokenizer, 1024-token input, and a bounded CPU pool. API and hybrid rules remain. [PR #8](https://github.com/MylesMCook/grasshopper/pull/8) merged as `58b670a`; marketplace `ede417f`. Synthetic hybrid recall is 122/160 top-1 and 143/160 top-3; longer handoffs improve from BGE's 2/8 and 3/8 to 7/8 and 8/8. The private 16-query shadow improves top-1 from 12 to 13 while retaining all top-three results. Three-OS real-model CI, Mac tests/vet/race, paired Thermos reviews, 289 archive hashes, and eight published asset digests pass. BGE/Granite restore and post-switch write/correction rollback rehearsals pass. The 2.4.0 Mac cutover preserved 20 records, 51 revisions/receipts, three device tokens, credentials, policy, and routing. Old BGE assets/state/configuration and checked encrypted off-host backups remain. [Evidence](docs/memory-acceptance.md#240-granite-recall-upgrade-september-29). No implementation or delivery work remains in this scope.

Direct installed adapter/bridge probes passed for Mac Codex, Cursor, and Claude; fresh native AI turns and desktop apps were not repeated. Other machines, reboot persistence, and full machine-loss recovery were not tested for 2.4.0.

## Released audit fixes

[LAB-216](https://linear.app/mcook/issue/LAB-216/fix-the-four-verified-grasshopper-thermos-audit-defects): [2.3.7](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.7) ships all four fixes from `1a52de3`; release commit `2e53204`, marketplace `0fe6ebf`. Three-OS CI, 280 package hashes, eight published asset digests, real pinned BGE tests, restored-backup reads, live local/Tailnet reads, and fresh no-tool Mac Codex/Cursor startup checks passed. The Mac service, public site, and Mac connectors are updated. Existing data and credentials remain; rollback binaries/plist/snapshot and a checked encrypted off-host backup are retained. [Evidence](docs/memory-acceptance.md#237-audit-fixes-september-29). No implementation or delivery work remains in this scope.

Other machines were not updated by this closeout. Mac Claude's 2.3.7 plugin installation was verified; a fresh authenticated Claude turn, desktop apps, and reboot persistence were not tested.

## Earlier released behavior

Released [2.3.6](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.6): one device connection is reused by installed agents. New connections return the owner-approval link immediately and finish on the next call. The private Mac service, public setup page, and Mac/Beelink connectors run 2.3.6. Existing credentials and the one SQLite database remain. Release commit `e61177a`; marketplace `55b69a0`. Three-OS CI, seven package checks, synthetic approval, backup restore, live service, fresh CLI context, cross-machine correction/readback, and bounded outage checks passed. [Evidence](docs/memory-acceptance.md#236-agent-connection-september-27).

Mac Cursor Agent CLI 2026.09.26-dd393fe passed a fresh 2.3.6 context turn with no tool events. [LAB-213](https://linear.app/mcook/issue/LAB-213/connect-grasshopper-once-across-codex-and-cursor) is complete. Mac Claude Code is logged out. Work HP, desktop apps, first-time hosting, reboot persistence, and full machine-loss recovery were outside this change.

[LAB-214](https://linear.app/mcook/issue/LAB-214/make-grasshopper-code-comments-accurate-and-useful): [PR #7](https://github.com/MylesMCook/grasshopper/pull/7) merged as `1d99bf5`. The review finding was addressed; production changes are comments only. Go tests, vet, race checks, JavaScript syntax, formatting, and three-OS CI passed.
