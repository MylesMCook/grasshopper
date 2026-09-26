# Grasshopper release state

Owner: Codex · Mac mini · `main` (2.3.2 client update released).

- [2.3.2](https://github.com/MylesMCook/grasshopper/releases/tag/v2.3.2) limits a stalled startup memory request to two seconds. [CI](https://github.com/MylesMCook/grasshopper/actions/runs/36246295264) passed on Mac, Linux, and Windows; all seven archives passed embedded hashes. The [marketplace](https://github.com/MylesMCook/grasshopper/tree/marketplace) is `25bd3cb`.
- Mac mini and Beelink Codex, Cursor, and Claude clients are updated to 2.3.2. Work HP Codex and Cursor are updated by CLI; Claude was not installed there. All three machines authenticated with the new package, and direct startup hooks loaded context. Fresh model turns were previously observed on 2.3.1, not repeated for 2.3.2. Work HP Cursor CLI first-turn injection remains unresolved; see [observed behavior](docs/memory-acceptance.md).
- Mac mini still runs compatible server 2.3.0 on its existing database and Tailnet route; no server restart or data migration was needed. The [public setup page](https://usegrasshopper.com/setup/) now serves 2.3.2 links (Cloudflare deployment `898d00cf-095a-4625-9b30-438766fc0504`).
- A stalled synthetic backend returned the unavailable startup fallback in 2.02 seconds. `go test ./...`, `go vet ./...`, and `go test -race ./internal/goclient` pass. The broader 5–8 second Mac agent-turn latency is mostly outside the hook; reasoning-effort quality on harder work is untested.

Still open: reliable Work HP Cursor first-turn injection, current Codex Desktop and Cursor IDE checks, reboot persistence, compaction/subagent refresh, and actual host failover. Do not infer app behavior from CLI checks.
