# Verification

What has been checked on real machines, what every release must pass, and what
has never been verified. Per-release detail lives in each
[GitHub release](https://github.com/MylesMCook/grasshopper/releases) and in Git
history (the full log up to 2.8.5 is in
[the 2.8.5 tree](https://github.com/MylesMCook/grasshopper/blob/v2.8.5/docs/memory-acceptance.md)).

## Clients verified on real machines

A fresh native CLI session received a confirmed memory in startup context before
any tool call. Each cell is the last version where that passed.

| Machine | Codex CLI | Cursor Agent CLI | Claude Code CLI |
| --- | --- | --- | --- |
| macOS (Apple silicon) | 2.3.4 | 2.3.6 | 2.3.4 |
| Linux (x64) | 2.3.4 | 2.3.4 | 2.3.4 |
| Windows (x64) | 2.6.0 | Not delivered in 2.3.3 | Not tested |

Since then, each release runs the installed macOS connectors' startup hooks and
five-tool bridges directly (latest: 2.8.5), without a fresh AI session.
Cross-machine save, correct and read back through one server passed between
macOS and Linux in 2.3.4. Desktop apps (Codex Desktop, Cursor IDE, Claude
desktop) have not been retested since 2.3.0.

## Every release

- `go test ./...`, `go vet ./...`, race tests and all Node tests pass locally.
- CI passes on macOS, Linux and Windows, including the real Granite model tests.
- All seven archives pass their embedded hash checks, and every published file
  matches `SHA256SUMS`.
- Packaged `--quickstart` starts, signs in and shuts down cleanly.
- A restored copy of a real database opens under the new server: current and
  earlier revisions read correctly, owner sign-in works and search returns
  results. A separate copy rolls back to the previous release.
- After the live cutover, record, revision, receipt and credential counts match
  the snapshot taken before it, and an encrypted off-host backup is verified
  before and after.
- The public site serves every asset, and each setup command copies exactly for
  all three operating systems.

## Never verified

- Physical phones, screen readers, and browsers other than Chromium.
- Reboot persistence of the server service, and full machine-loss recovery.
- Persistent Windows or Linux server deployments (only macOS runs one).
- Native context refresh after compaction or in subagents.
- macOS Gatekeeper and Windows SmartScreen behavior for the unsigned archives.

Unit tests and CI do not prove any of these. Record new real-machine results in
this file: harness and version, OS, what was observed, and what was not.
