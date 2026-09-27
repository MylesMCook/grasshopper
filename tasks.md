# Grasshopper release state

Owner: Codex · Mac mini · main. Active outcome: [LAB-213](https://linear.app/mcook/issue/LAB-213/connect-grasshopper-once-across-codex-and-cursor).

Connect once across Codex and Cursor: reuse a saved connection, accept normal server/view/MCP links on a new machine, show focused owner approval, and preserve working configuration through offline or failed replacement attempts. The one canonical policy and private backend remain. Work HP and first-time server hosting are outside this change.

Implemented locally, uncommitted: link normalization and connection states; no-repeat connect and explicit reconnect/switch behavior; focused approval UI; shorter plugin, README, and website instructions. Go tests, vet, race checks, and synthetic browser approval pass. An extracted 2.3.5 client paired, authenticated, and reused its credential without rewriting it.

Next: finish three-OS release packages and checksums, rehearse private-service restore, then update Mac mini and Beelink with rollback. Verify fresh native CLI turns and public site before marking LAB-213 done.

## Code-comment transparency

Owner: Codex · Mac mini · `/Users/mylescook/.codex/worktrees/grasshopper-code-comments/grasshopper` · `codex/code-comment-transparency`. Outcome: [LAB-214](https://linear.app/mcook/issue/LAB-214/make-grasshopper-code-comments-accurate-and-useful).

Implemented on this branch: a root code-comment rule and a production comment audit across commands, storage, MCP, client, embeddings, viewer, and integration scripts. The canonical memory policy is unchanged. Rebased on the connection code commit `66eba94`, then checked its new declarations and comments. Full Go tests, vet, storage/server/client race tests, JavaScript syntax, gofmt, and a comment-free Go AST comparison passed after the rebase. Real BGE model tests were not run.

Ready for review: [PR #7](https://github.com/MylesMCook/grasshopper/pull/7) has passed CI on macOS, Ubuntu, and Windows. Next: human review. Keep LAB-214 in review until the PR is accepted; do not merge it in this task.
