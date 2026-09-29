# Grasshopper release state

Owner: Codex · Mac mini · `/Users/mylescook/Code/MylesMCook/grasshopper` · `main`.

## Released model upgrade

[LAB-217](https://linear.app/mcook/issue/LAB-217/improve-memory-recall-with-a-verified-model-upgrade-and-reversible): [2.4.0](https://github.com/MylesMCook/grasshopper/releases/tag/v2.4.0) ships Granite small English R2 with pinned graph/weights/tokenizer, 1024-token input, and a bounded CPU pool. API and hybrid rules remain. [PR #8](https://github.com/MylesMCook/grasshopper/pull/8) merged as `58b670a`; marketplace `ede417f`. Synthetic hybrid recall is 122/160 top-1 and 143/160 top-3; longer handoffs improve from BGE's 2/8 and 3/8 to 7/8 and 8/8. The private 16-query shadow improves top-1 from 12 to 13 while retaining all top-three results. Three-OS real-model CI, Mac tests/vet/race, paired Thermos reviews, 289 archive hashes, and eight published asset digests pass. BGE/Granite restore and post-switch write/correction rollback rehearsals pass. The Mac service and connectors run 2.4.0; cutover preserved 20 records, 51 revisions/receipts, three device tokens, credentials, policy, and routing. Old BGE assets/state/configuration and checked encrypted off-host backups remain. [Evidence](docs/memory-acceptance.md#240-granite-recall-upgrade-september-29). No implementation or delivery work remains in this scope.

Direct installed adapter/bridge probes passed for Mac Codex, Cursor, and Claude; fresh native AI turns and desktop apps were not repeated. Other machines, reboot persistence, and full machine-loss recovery were not tested for 2.4.0.

## Released audit fixes

[LAB-216](https://linear.app/mcook/issue/LAB-216/fix-the-four-verified-grasshopper-thermos-audit-defects): [2.3.7](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.7) ships all four fixes from `1a52de3`; release commit `2e53204`, marketplace `0fe6ebf`. Three-OS CI, 280 package hashes, eight published asset digests, real pinned BGE tests, restored-backup reads, live local/Tailnet reads, and fresh no-tool Mac Codex/Cursor startup checks passed. The Mac service, public site, and Mac connectors are updated. Existing data and credentials remain; rollback binaries/plist/snapshot and a checked encrypted off-host backup are retained. [Evidence](docs/memory-acceptance.md#237-audit-fixes-september-29). No implementation or delivery work remains in this scope.

Other machines were not updated by this closeout. Mac Claude's 2.3.7 plugin installation was verified; a fresh authenticated Claude turn, desktop apps, and reboot persistence were not tested.

## Earlier released behavior

Released [2.3.6](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.6): one device connection is reused by installed agents. New connections return the owner-approval link immediately and finish on the next call. The private Mac service, public setup page, and Mac/Beelink connectors run 2.3.6. Existing credentials and the one SQLite database remain. Release commit `e61177a`; marketplace `55b69a0`. Three-OS CI, seven package checks, synthetic approval, backup restore, live service, fresh CLI context, cross-machine correction/readback, and bounded outage checks passed. [Evidence](docs/memory-acceptance.md#236-agent-connection-september-27).

Mac Cursor Agent CLI 2026.09.26-dd393fe passed a fresh 2.3.6 context turn with no tool events. [LAB-213](https://linear.app/mcook/issue/LAB-213/connect-grasshopper-once-across-codex-and-cursor) is complete. Mac Claude Code is logged out. Work HP, desktop apps, first-time hosting, reboot persistence, and full machine-loss recovery were outside this change.

[LAB-214](https://linear.app/mcook/issue/LAB-214/make-grasshopper-code-comments-accurate-and-useful): [PR #7](https://github.com/MylesMCook/grasshopper/pull/7) merged as `1d99bf5`. The review finding was addressed; production changes are comments only. Go tests, vet, race checks, JavaScript syntax, formatting, and three-OS CI passed.
