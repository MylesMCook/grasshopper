# Grasshopper UX audit: hosting and recovery

Owner: Codex (gpt-6.1-sol), Mac mini. Isolated checkout
`/Users/mylescook/.codex/worktrees/ux-audit-report3-server/grasshopper`, branch
`codex/ux-audit-hosting`, base `b3a4f46`. Local changes only. Root owns integration,
Linear, release and final verification. No live service, credential, route or
supervisor changed.

The supplied audit outcomes are accepted requirements. Native acceptance tests
use synthetic databases and credentials. The prior report 3 checkpoint remains
in [its report](ux-audit-report3-server-results.md).

## R6-1: Private proxy hosting

**Seen:** quickstart rejected `--allowed-proxy-host`; owner sign-in accepted an
unconfigured Host with its matching HTTP Origin.

**Impact:** the documented private HTTPS path was unavailable through quickstart,
and browser owner APIs lacked an explicit trusted-host boundary.

**Fix:** quickstart accepts the exact proxy flag without saving it into state.
MCP and browser owner APIs allow loopback/localhost or the configured Host only.
Rejected Host/Origin errors explain the configured private proxy setting without
reflecting attacker-supplied header values. README gives exact startup flags and
the existing private Serve forwarding target, without changing a route.

**Check / verified:** sign-in regression first returned 204 for a wrong Host and
HTTP Origin; it now returns 403. Matching proxy Host/HTTPS Origin signs in with
204. Wrong scheme, Host or Origin returns 403. Existing MCP proxy test verifies
200 for the configured Host/Origin and 403 for wrong Host/Origin. Quickstart flag
parsing reaches bundle validation instead of rejecting proxy flags.

**Default-port correction:** integration review found that the original tests
used explicit `:443` in both headers, while browsers normally omit it. Added
regressions first failed with 403 for omitted Host/Origin ports. Matching now
accepts both equivalent HTTPS-default-port forms consistently for host guards,
owner origins, Secure cookies, pairing origins and MCP proxy translation.
Wrong hostname, nondefault port, HTTP scheme, userinfo and path variants remain
rejected. Full gomcp tests, vet and race checks pass after correction.

**Not tested:** an actual Tailscale route, off-machine browser or deployment.

## R6-4: Persistent server templates

**Seen:** launchd's binary path used the source name absent from the archive.

**Impact:** copying the shipped template could leave the service unable to start.

**Fix:** template uses `bin/grasshopper-server`, includes an optional commented
proxy flag, and the operations guide explains private log-directory creation,
launchctl bootstrap/bootout, a systemd user unit and Windows Task Scheduler.

**Check / verified:** plist lint passes. Synthetic archive construction verifies
that its plist resolves to the bundled server binary. Documentation retains
loopback binding, one writer and stopped-server update requirements.

**Not tested:** supervisor installation, login startup or reboot persistence.

## R6-3: Unsigned download and executable guidance

**Seen:** archive opening guidance did not explain unsigned executable warnings.

**Impact:** owners could mistake platform warnings for a broken download or use
unnecessary machine-wide security changes.

**Fix:** README requires trusted-source/hash verification, links Apple's supported
Privacy & Security opening flow and Microsoft's SmartScreen guidance, and says
the executables are unsigned. Source-build instructions use explicit archive
binary names. No security bypass or signing infrastructure was executed.

**Check / verified:** actual local `go build -o` produced `grasshopper`,
`grasshopper-server`, `grasshopper-backup` and `grasshopper-migrate` in task-local
scratch. Official Apple opening and Microsoft SmartScreen source references were
checked through current official search results; Tailscale Serve docs opened.

**Not tested:** a quarantined downloaded archive, Windows SmartScreen, Windows
ACL enforcement or Windows filesystem replacement guarantees. Unix tests check
0600 credentials. OS download UI behavior remains a platform acceptance gap.

## R6-5: Owner token loss and leak recovery

**Seen:** no supported offline owner-token replacement command existed.

**Impact:** loss blocked owner access; a leaked token had no guided replacement
lifecycle that distinguished a running server's cached credential.

**Fix:** `--rotate-token --server-stopped --data-dir /absolute/private/state`
requires a stopped-server acknowledgement and an existing regular database in a
private, unlinked directory. It writes/syncs a private temporary token and renames
it over `access-token`, syncs the containing directory on Unix, keeping an exclusive private `access-token.previous`
rollback credential. A lost token can be recreated. An existing rollback file is
never overwritten. Output and operations guidance require restart and explain
that old owner credentials/cookies remain applicable until reload. No process is
stopped automatically; `--server-stopped` is an acknowledgement, not a lock or
process detector. Remove a leaked rollback credential after verification.

**Check / verified:** tests cover refusal without acknowledgement, lost-token
recovery, replacement, private credential modes, retained rollback, refusal to
overwrite rollback, database byte preservation, and valid paired-device tokens.
An already-running handler still accepts the old owner token after file rewrite;
a handler reloaded with the new token rejects the old token and cookie and
accepts new owner sign-in. No model is needed for offline rotation.

**Not tested:** live rotation, power-loss recovery or cross-process races. The
operator must stop every server first. Root's parallel data/session slice must
preserve the owner-token binding verified by this test during integration.

## R5-5: Backup verification and recovery

**Seen:** backup completion did not display the integrity and identity evidence
needed to evaluate a recovery snapshot.

**Impact:** owners could select an empty, corrupt or accidentally shared file.

**Fix:** backup and read-only `--verify PATH` print current memory count,
historical revision count, SQLite quick_check, byte size and SHA-256. Verification
fails for empty/corrupt/nonprivate/nonregular files and nonempty WAL/journal sidecars. README gives exact independent
verification and stopped-server copy/recovery steps without a destructive restore
command. SnapshotCounts uses the read-only SQLite reader.

**Check / verified:** synthetic snapshot tests verify counts/integrity/digest and
byte preservation, reject empty/corrupt/shared files and pending sidecars, and existing backup tests
preserve source state and prevent destination overwrite.

**Not tested:** restoring a live database, off-host transfer or machine loss.
Pending-WAL regression first verified a sidecar-bearing file; it now rejects it
with snapshot-first guidance. Counts/digest establish snapshot evidence; they do not prove restore acceptance.

## R5-6: Actionable startup errors

**Seen:** path/startup errors lacked the relevant command flag context.

**Impact:** owners could not identify whether the database, token or embedding
configuration needed correction.

**Fix:** token failures name `--token-file`; embedding initialization names
`--onnx-library`, `--model` and `--tokenizer` while preserving Granite's narrower
asset errors; opening the database names `--db`.

**Check / verified:** missing token and model tests assert flag context. Existing
server tests pass.

**Not tested:** each real ONNX initialization failure or live startup failure.

## R4-6: Release download selection

**Seen:** releases lacked a concise reusable file-selection guide.

**Impact:** normal marketplace users could download unnecessary archives.

**Fix:** checked-in release notes template distinguishes hosting, ordinary
marketplace installation, offline client installation and marketplace maintenance;
links backup/update/recovery. Names match the release tooling and read-only
v2.7.0 asset listing. Published release content was not changed.

**Check / verified:** actual release filenames and build-script naming inspected.

**Not tested:** a future release rendered using the template.

## R6-8: Server archive contents

**Seen:** server archives shipped obsolete manual client-wiring examples.

**Impact:** owners could follow an older installation path alongside the normal
connector flow.

**Fix:** remove integrations examples from server archive construction only.
Repository fixtures and client archive policy/examples remain. Include operations
guidance so the server README's hosting/recovery links resolve in the archive.

**Check / verified:** synthetic server archive rejects any integrations entry,
resolves launchd binary, and existing three-harness client archive tests pass.

**Not tested:** full real-model native archive startup in this worker run.

## R3-9: Browser sign-in versus device connection

**Seen:** prior report required final terminology verification against site work.

**Impact:** browser authentication could be confused with agent/device pairing.

**Fix / source review:** read-only integration source at `f7179bf` presents browser
`Sign in`/`Signed out` and distinguishes owner and device tokens; public `/view/`
opens the owner server without collecting a token. Setup says `Connect your
agents` and `Connect once`; connected-device controls remain device terminology.
No frontend file was changed in this slice.

**Check / verified:** source inspection of public view/setup and private viewer.
The setup phrase `already-connected memory view` remains a minor copy suggestion
for the frontend owner: `already-signed-in memory view` would be clearer.

**Not tested:** combined browser flow or native agent session.

## Checks run

- `go test ./cmd/grasshopper-go-server ./cmd/grasshopper-go-backup ./cmd/grasshopper-go-bundle ./internal/gomcp ./internal/gomemory`
- `go vet` for those same packages.
- `plutil -lint packaging/macos/com.example.grasshopper.plist`
- Explicit local binary alias builds for client/server/backup/migrate.
- `go test -race` for server/backup/gomcp/gomemory.
- `git diff --check`

Model-dependent native bundle test was skipped because its required test asset
variables were not configured; no real-model pass is claimed. Root owns combined
verification and any later publication or deployment.
