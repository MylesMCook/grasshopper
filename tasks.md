# Grasshopper release state

Owner: Codex · Mac mini · main. Active outcome: [LAB-213](https://linear.app/mcook/issue/LAB-213/connect-grasshopper-once-across-codex-and-cursor).

Connect once across Codex and Cursor: reuse a saved connection, accept normal server/view/MCP links on a new machine, show focused owner approval, and preserve working configuration through offline or failed replacement attempts. The one canonical policy and private backend remain. Work HP and first-time server hosting are outside this change.

Released 2.3.5: main `66eba94`, marketplace `1a0b6f1`, [GitHub release](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.5). The private Mac service and public setup site run 2.3.5. Mac and Beelink Codex/Cursor wiring and Claude plugins are updated; saved device credentials were reused. Synthetic browser pairing, three-OS CI, seven package hashes, Mac restore/rollback rehearsal, and live backup passed. Fresh Mac/Beelink Codex and Beelink Cursor/Claude received context; Beelink Cursor corrected a synthetic record and Mac Codex read current/prior revisions. See [evidence](docs/memory-acceptance.md#235-connection-flow-september-27).

In progress: a fresh Codex CLI sandbox test found that blocked network access was mislabeled as an offline server. Version 2.3.6 now reports a native network-permission step; the unit and real macOS sandbox checks pass. Publish its coordinated packages and update clients before closing this outcome. The Mac login keychain remains locked, so Mac Cursor CLI cannot start for its fresh-turn check; Mac Claude Code is logged out. Work HP, desktop apps, and first-time hosting remain outside this outcome.
