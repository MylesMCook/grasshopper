# Grasshopper release state

Owner: Codex · Mac mini · `main` · [LAB-213](https://linear.app/mcook/issue/LAB-213/connect-grasshopper-once-across-codex-and-cursor).

Released [2.3.6](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.6): one device connection is reused by installed agents. New connections return the owner-approval link immediately and finish on the next call. The private Mac service, public setup page, and Mac/Beelink connectors run 2.3.6. Existing credentials and the one SQLite database remain. Main `e61177a`; marketplace `55b69a0`. Three-OS CI, seven package checks, synthetic approval, backup restore, live service, fresh CLI context, cross-machine correction/readback, and bounded outage checks passed. [Evidence](docs/memory-acceptance.md#236-agent-connection-september-27).

Remaining: Mac Cursor Agent CLI needs the login keychain unlocked for a fresh 2.3.6 model turn; Mac Claude Code is logged out. Work HP, desktop apps, first-time hosting, reboot persistence, and full machine-loss recovery were outside this change. Next: test Mac Cursor CLI after the keychain is unlocked, then reconcile LAB-213.
