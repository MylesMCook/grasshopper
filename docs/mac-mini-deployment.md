# Mac mini private deployment

**Prepared, not installed.** The current Mac mini has no persistent Grasshopper job or Grasshopper private route. A separate synthetic pilot is running on loopback; do not stop or reuse it. The first deployment should use a fresh Go database. An old-data migration can follow only after its own snapshot review. This plan creates one writer on `127.0.0.1:8106` and one new Tailscale Serve HTTPS mapping. It changes no existing Serve ports.

## Before approval

The proposed state directory is `~/Library/Application Support/Grasshopper` with private `app`, `data`, `secrets`, `backups`, and `logs` subdirectories. The proposed launchd job is `com.myles.grasshopper`; the proposed tailnet-only HTTPS port is `8456`. Recheck the port and Serve map immediately before applying. The checked Mac had no Time Machine destination and no Grasshopper off-host backup. **Choose and verify an independent backup destination before treating this as recoverable.** A local SQLite snapshot protects against a bad migration, not loss of the Mac.

The owner is the Mac mini operator. The release gate is a second-device HTTPS read/write, a verified restored backup, and all three CLIs reading the same record. A service restart or local `200` alone does not pass. Clients use bounded timeouts and idempotent request IDs; do not retry an uncertain write with a new ID.

Keep the bearer token in a mode-`0600` file outside `app`; keep the parent mode `0700`. The token grants access to the entire personal store. This route must stay on Tailscale Serve, never Funnel. Keep employer-restricted records in a separate access boundary.

## Build and stage

Use a checksum-verified macOS arm64 full bundle. If building from source on the Mac, build the native server and backup commands, then pass them with the pinned model, tokenizer, ONNX Runtime library, license, and notices to `grasshopper-go-bundle` (see [portable package](go-package.md)). Verify every `SHA256SUMS` entry before copying the archive's `bin`, `models`, and `runtime` directories into `app`. Keep a copy of the prior `app` directory for binary rollback. Do not package or log the token or database.

For a new installation, create an empty `data/memory.db` with `grasshopper-go-server --create-db` only on its first start, then stop that one-shot process before loading launchd. The launchd example omits `--create-db` so a missing database cannot silently become an empty new one. For a later migration, stop the sole old writer, take a consistent snapshot with `grasshopper-go-backup`, convert a separate copy with `grasshopper-go-migrate`, and verify records and legacy quarantine as in [migration and recovery](shared-memory.md). Never point an older Rust binary at a Go-written database.

Copy [the launchd example](../packaging/macos/com.example.grasshopper.plist) to `~/Library/LaunchAgents/com.myles.grasshopper.plist`. Replace its example username and label; check every binary, model, database, token, and log path. Run `plutil -lint` on the rendered plist. Its executable reads a token file, binds loopback, serves the optional [live memory view](memory-visualizer.md), restarts after failure, and writes logs under the private state directory. It contains no token value.

## Approved cutover sequence

1. **Snapshot and restore.** With the old writer stopped, run `grasshopper-go-backup --source SOURCE.db --dest NEW-BACKUP.db`. Check `PRAGMA integrity_check`, copy the backup to a new path, and read the restored records through a Go server. Preserve the original database and any WAL/SHM files until rollback is no longer needed. Record the latest revision before switching writers.
2. **Start locally.** Load the rendered job with `launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.myles.grasshopper.plist`. Check `launchctl print gui/$(id -u)/com.myles.grasshopper`, `lsof -nP -iTCP:8106 -sTCP:LISTEN`, and recent private logs. An unauthenticated `/healthz` request must return `401`; an authenticated request must return `200`. Make a synthetic scoped write, then read it back by ID and revision.
3. **Open only the private route.** After confirming port `8456` is unused, run `tailscale serve --bg --https=8456 http://127.0.0.1:8106`. Confirm `tailscale serve status` shows only the new mapping and the previous mappings unchanged. From a second tailnet device, use the HTTPS URL with the client token file to read and write the same synthetic record. An unauthenticated remote request must return `401`.
4. **Check recovery and clients.** Make a fresh Go backup after the synthetic write and reopen it at another path. Then test Codex, Cursor, and Claude CLIs on their intended machines with the same backend, including a bounded outage. Record versions, OS, IDs/revisions, and limits in [acceptance evidence](memory-acceptance.md). Do not call this deployed until the remote check passes.

Tailscale's [Serve command](https://tailscale.com/docs/reference/tailscale-cli/serve) provides HTTPS for tailnet devices and can remove one mapping with its matching `off` command. [Funnel](https://tailscale.com/docs/features/tailscale-funnel) is public and is not part of this rollout.

## Stop or roll back

Remove only Grasshopper's route with the matching `tailscale serve --bg --https=8456 http://127.0.0.1:8106 off`; confirm all older Serve mappings remain. Never use `tailscale serve reset` here. Stop only the Grasshopper job with `launchctl bootout gui/$(id -u)/com.myles.grasshopper`. If an update failed, restore the previous verified `app` files and relaunch the same Go job. Do not overwrite the active database.

If data restoration is required, first preserve the current database **and** its WAL/SHM while the writer is stopped. Restore a verified Go backup to a *new* path and launch a Go server against that copy. Compare IDs and revisions before changing clients. A backup taken before accepted writes omits those writes; reconcile them from the preserved post-cutover database before declaring recovery complete.

For routine backups, use a new destination each run. Verify SQLite integrity and a read from the restored copy, then transfer the encrypted backup to the approved independent destination. Prune only after retention and restore checks are approved. Check log size; launchd's `ThrottleInterval` limits rapid restart, not log growth.

## Evidence so far

On the Mac mini, a task-local Go server passed `401` without a token and `200` with one. A confirmed synthetic preference was written at revision 1. `grasshopper-go-backup` captured it while the server ran. After a revision-2 correction on the source, a server opened from the backup returned revision 1 through `get` and `context`; SQLite `integrity_check` returned `ok`. This proves a consistent local snapshot and readback, not off-host recovery, reboot persistence, or the private route.
