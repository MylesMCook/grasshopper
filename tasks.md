# Grasshopper release state

## UX audits 1–6: verified, preparing PR

Codex · Mac mini · `/Users/mylescook/.codex/worktrees/ux-audit-integration/grasshopper`
· `codex/ux-audit-integration`. Integrated released 2.7.0 evidence `2031446` and
all audit worker slices through `9d681aa`. Primary checkout and live service were
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
preview at743px, pinned maximum-text dialog,103 unique paginated records, dirty
protection, URL reload, attribution, archive Undo, revision restoration, complete
JSON download, sign-out everywhere and automatic failure recovery. Local public
build passes wrapping, selected-OS clipboard, metadata/favicon/theme/screenshot.
Review defects were fixed and retested. No new dependencies/framework.

Next: push the verified branch, create the single PR from the owner's accepted
plan, verify CI and merge. Release/public-site deployment/live-host changes are
separate and have not occurred. Physical devices, screen readers, native
Windows/Linux, Tailscale Serve/reboot and fresh deployed agent turns remain
untested. Task-local evidence is outside Git or under untracked `output/`.

Owner: Codex · Mac mini · `/Users/mylescook/Code/MylesMCook/grasshopper` · `main`.

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

[LAB-218](https://linear.app/mcook/issue/LAB-218/search-and-inspect-saved-memories-in-the-memory-view): [2.5.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.5.0) ships owner-only scoped search and full inspection of saved memories, including omitted large records, confirmation/date/source, and revision history. Release commit `2bd4927`; marketplace `160f630`. Granite, ranking, startup selection, stored records, credentials, and the five MCP tools remain unchanged. Paired Thermos review, three-OS real-model CI, package digests, synthetic desktop/mobile browser checks with real Granite semantic search, Mac rollback and live deployment, authenticated local/Tailnet reads, off-host backup, and direct Mac Codex/Cursor/Claude connector probes passed. [Evidence](docs/memory-acceptance.md#250-memory-view-search-and-inspection-september-29). No delivery work remains for this slice. Later product candidates are correction/archive controls, startup-budget allocation, and ranking trials; they have not been implemented. Research: `/Users/mylescook/Documents/Codex/2026-09-29-grasshopper-product-review/PRODUCT-REVIEW.md`.

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
