# Grasshopper 2.0.0 release

Owner: Codex. Mac checkout: `/Users/mylescook/Code/MylesMCook/grasshopper`,
branch `main`. Work HP checks are CLI-only while the user works there. Beelink
is shared with a separate host-maintenance task. Preserve unrelated `.build/`
and `.playwright-cli/` folders.

## Intended behavior

One authenticated Go/SQLite memory service; one stateless Go bridge for Codex,
Cursor, and Claude; one shared `integrations/policy/AGENTS.md`. The five MCP
tools remain `context`, `store`, `search`, `get`, and `archive`. A bundled,
read-only live view shows scoped context without another store. Rust and local
code search are retired.

## Verified

- Backend tests, real BGE recall, copy-only migration, backup/restore, and
  earlier portable bundles passed on Mac, Windows, and Ubuntu. Package CI for
  `38305a0` and Windows fallback CI for `f27bc8f` passed on all three OSes.
- Fresh Codex, Cursor, and Claude CLI turns read synthetic memory on all three
  machines. A later shared-server pilot used one Mac backend: all three OSes
  read global record 1 revision 2. Direct bridge reads kept project A `bun`,
  project B `npm`, and a Windows path separate. All pilot forwards, listeners,
  and token copies were removed.
- Work HP Codex CLI fresh/second/resume turns and Claude fresh/resume/subagent/
  compact hooks passed their recorded checks. Mac Cursor CLI needed project MCP
  wiring; plugin-only loading did not register tools.
- Mac Chromium showed a new memory in the live view without reload, a clear
  stale state during outage, and no 390px horizontal overflow. Go tests, vet,
  race checks, and the example Mac launchd plist pass.
- Native 2.0.0 candidates passed real BGE tests, vet, archive checksums, and
  extracted-server auth/live-view reads on Mac, Windows, and Linux. A package
  review found broken guide links; the fixed Mac candidates now pass link and
  checksum checks. Isolated Codex and Claude install/update/removal passed on
  all three OSes. Windows/Linux need one final rebuild from the fix.
- Synthetic Mac online backup restored revision 1 after the source advanced to
  revision 2. No live service or data was changed. See
  [acceptance evidence](docs/memory-acceptance.md).

## Release gates

- Build final 2.0.0 Mac, Windows, and Linux client/server archives from this
  commit. Verify checksums, fresh install/update/removal, auth, live view,
  startup context, and stopped-backend behavior. Run final CI and review.
- Tag and publish the software release after those gates pass. Keep a clear
  limitation for Cursor native Git marketplace and desktop behavior.
- Mac mini is the chosen first persistent host. [Deployment plan](docs/mac-mini-deployment.md)
  is prepared, not applied. Independent off-host backup and explicit
  supervisor/private-route approval are required before a live service. Do not
  migrate live data as part of the software release.
