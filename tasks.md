# Grasshopper release state

Owner: Codex · Mac mini · `/Users/mylescook/Code/MylesMCook/grasshopper` · `codex/thermos-audit-fixes`.

## Audit release closeout

[LAB-216](https://linear.app/mcook/issue/LAB-216/fix-the-four-verified-grasshopper-thermos-audit-defects): fixed snapshot privacy during copying, Cursor hook quoting, connection-option validation/recovery, and stale device polling in `1a52de3`. Regression checks failed before the fixes and passed afterward. Full Go tests, vet, scoped race checks, five binary builds, and four Node regressions passed. Two fresh Thermos reviewers found no remaining defects. A real browser verified cancellation, synthetic device revocation, and stopped polling after disconnect. [Evidence](docs/memory-acceptance.md#local-audit-repairs-september-29). The owner requested git-it-out: land main, pass three-OS CI, publish 2.3.7 and marketplace packages, verify a restored backup, update the private Mac service/public site and local connector, then record delivery evidence.

## Released behavior

Released [2.3.6](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.6): one device connection is reused by installed agents. New connections return the owner-approval link immediately and finish on the next call. The private Mac service, public setup page, and Mac/Beelink connectors run 2.3.6. Existing credentials and the one SQLite database remain. Release commit `e61177a`; marketplace `55b69a0`. Three-OS CI, seven package checks, synthetic approval, backup restore, live service, fresh CLI context, cross-machine correction/readback, and bounded outage checks passed. [Evidence](docs/memory-acceptance.md#236-agent-connection-september-27).

Mac Cursor Agent CLI 2026.09.26-dd393fe passed a fresh 2.3.6 context turn with no tool events. [LAB-213](https://linear.app/mcook/issue/LAB-213/connect-grasshopper-once-across-codex-and-cursor) is complete. Mac Claude Code is logged out. Work HP, desktop apps, first-time hosting, reboot persistence, and full machine-loss recovery were outside this change.

[LAB-214](https://linear.app/mcook/issue/LAB-214/make-grasshopper-code-comments-accurate-and-useful): [PR #7](https://github.com/MylesMCook/grasshopper/pull/7) merged as `1d99bf5`. The review finding was addressed; production changes are comments only. Go tests, vet, race checks, JavaScript syntax, formatting, and three-OS CI passed.
