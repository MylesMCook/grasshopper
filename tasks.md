# Grasshopper release state

Owner: Codex · Mac mini · `/Users/mylescook/Code/MylesMCook/grasshopper` · `codex/device-view`.

## Active memory-view simplification

[LAB-220](https://linear.app/mcook/issue/LAB-220/simplify-the-memory-view-layout-and-filter-flow): user requested a simpler private memory view and authorized deployment and Git closeout, including LAB-219. Owner remains Codex in this checkout. Search and native scope selectors now share one quiet surface; the exact selected scope precedes results; device management is secondary. Presentation changes preserve search, authentication, retrieval, and history. Research: Nielsen Norman Group progressive disclosure and GOV.UK native select guidance, evaluated through Laws of Taste and Laws of Software. Browser checks and release are in progress.

## Active device-view fix

[LAB-219](https://linear.app/mcook/issue/LAB-219/show-connected-devices-accurately-in-the-memory-view): approved dropdown fix is implemented and verified on this local branch, not deployed. Both owner context/search include active paired IDs without scoped memories; scoped-memory choices remain after revocation; agent retrieval is unchanged. Regression failed before the fix and passed afterward; Go tests/vet/race, client/server builds, and 10 Node tests passed. [Evidence](docs/memory-acceptance.md#device-list-investigation-and-local-fix-september-29-unreleased).

Confirmed cause: native Windows Codex had a Store-app redirected config using the owner credential; ordinary Windows Roaming held a separate revoked `work-hp` connection. Approved repair completed: `HPLT2MQ5360JD8` is actively paired, its credential reads #8 revision 1, owner controls reject it with 401, and the installed Windows client check passes. Supported setup updated the packaged config and replaced its owner-token file; staged pairing files were removed. Other connections, Mac credentials/service/routing, and ordinary Windows config were untouched. Next user action: reopen active laptop chats to clear loaded owner credentials; a fresh native AI turn remains unverified. Dropdown deployment is now approved with LAB-220.

User direction: connectors should be minimal and reliable, with complexity on the host. Prevention target: one deterministic connection shared across harnesses, device-only agent credentials, and host-reported identity/status. A broader connection-flow change is not yet implemented or approved.

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
