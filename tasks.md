# Grasshopper release state

Owner: Codex · Mac mini · `main` · [LAB-213](https://linear.app/mcook/issue/LAB-213/connect-grasshopper-once-across-codex-and-cursor).

Released [2.3.6](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.6): one device connection is reused by installed agents. New connections return the owner-approval link immediately and finish on the next call. The private Mac service, public setup page, and Mac/Beelink connectors run 2.3.6. Existing credentials and the one SQLite database remain. Main `e61177a`; marketplace `55b69a0`. Three-OS CI, seven package checks, synthetic approval, backup restore, live service, fresh CLI context, cross-machine correction/readback, and bounded outage checks passed. [Evidence](docs/memory-acceptance.md#236-agent-connection-september-27).

Remaining: Mac Cursor Agent CLI needs the login keychain unlocked for a fresh 2.3.6 model turn; Mac Claude Code is logged out. Work HP, desktop apps, first-time hosting, reboot persistence, and full machine-loss recovery were outside this change. Next: test Mac Cursor CLI after the keychain is unlocked, then reconcile LAB-213.

[LAB-214](https://linear.app/mcook/issue/LAB-214/make-grasshopper-code-comments-accurate-and-useful): [PR #7](https://github.com/MylesMCook/grasshopper/pull/7) documents code contracts and adds a short comment rule. Its production changes are comments only. Reviewed against 2.3.6; Go tests, vet, race checks, JavaScript syntax, formatting, and the diff check pass. Awaiting PR CI and merge.
