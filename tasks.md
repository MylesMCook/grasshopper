# Grasshopper release state

Owner: Codex · Mac mini · main. Active outcome: [LAB-213](https://linear.app/mcook/issue/LAB-213/connect-grasshopper-once-across-codex-and-cursor).

Connect once across Codex and Cursor: reuse a saved connection, accept normal server/view/MCP links on a new machine, show focused owner approval, and preserve working configuration through offline or failed replacement attempts. The one canonical policy and private backend remain. Work HP and first-time server hosting are outside this change.

Implemented locally, uncommitted: link normalization and connection states; no-repeat connect and explicit reconnect/switch behavior; focused approval UI; shorter plugin, README, and website instructions. Go tests, vet, race checks, and synthetic browser approval pass. An extracted 2.3.5 client paired, authenticated, and reused its credential without rewriting it.

Next: finish three-OS release packages and checksums, rehearse private-service restore, then update Mac mini and Beelink with rollback. Verify fresh native CLI turns and public site before marking LAB-213 done.
