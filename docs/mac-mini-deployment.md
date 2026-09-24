# Mac mini private service

**Running with a fresh database.** Grasshopper listens on `127.0.0.1:8106` under the `com.myles.grasshopper` launchd job. Tailscale Serve maps private HTTPS port `8456` to that loopback listener. Funnel is off. Existing Serve mappings were compared before and after; none changed. No legacy database was migrated.

The private state is under `~/Library/Application Support/Grasshopper`: `app` holds the checksum-verified server bundle, `data` the sole writable database, `secrets` the bearer token and backup credentials, and `backups` and `logs` the local snapshots and service output. Keep these directories private. Never put a token, filled client config, or database in a release archive.

## Backup and recovery

An encrypted restic repository lives in a dedicated Cloudflare R2 bucket, `grasshopper-memory-backups`. Its key is limited to that bucket. The repository address, restic password, and scoped key pair are in the Bitwarden secure note **Grasshopper R2 backup recovery**; no values are in this repository. A separate `com.myles.grasshopper.backup-r2` launchd job runs daily at 03:15. Its private script makes a consistent SQLite snapshot, checks integrity, uploads it, checks the remote repository, and acknowledges only after upload. Retention and pruning are not configured.

The initial encrypted upload passed a full-data check. A restored copy matched the source bytes and passed SQLite integrity. A later backup after a synthetic write was restored to another path; a server read the saved record and its earlier revision from that copy. The backup job also passed an explicit launchd run. This proves off-host restore of synthetic data, not recovery from a lost Mac or reboot persistence. Check backup logs and repeat a restore periodically; do not infer backup health from the job being loaded.

To recover, retrieve the secure note from Bitwarden on a separate device, install restic, and restore to a **new** private directory. Check repository data, SQLite integrity, and record IDs/revisions before pointing any client at the copy. If the original Mac survives, stop the sole writer and preserve the current database with its WAL/SHM files first. A backup taken before an accepted write lacks that write; compare the preserved database before cutover. Keep only one active writer.

## Service and route checks

Check `launchctl print gui/$(id -u)/com.myles.grasshopper`, `lsof -nP -iTCP:8106 -sTCP:LISTEN`, private logs, and `tailscale serve status`. `/healthz` must return `401` without the bearer token and `200` with it. The MCP endpoint must also reject an unauthenticated request. From another tailnet device, verify a scoped read and write through HTTPS. Clients use bounded timeouts and idempotent request IDs; never retry an uncertain write with a new ID.

When using Tailscale Serve, set the server's `--allowed-proxy-host` to the **exact** private HTTPS host and port. This admits only that proxy Host to `/mcp`; every other Host keeps the SDK's loopback protection, and a mismatched Origin is rejected. Do not use a wildcard or expose the loopback service publicly. Keep employer-restricted records in a separate access boundary.

## Update or roll back

Verify a new [portable bundle](go-package.md) and its internal checksums before stopping the job. Save the prior app binary and rendered plist outside `data`. Update only Grasshopper's job, then check local auth, the private route, a full-record read, and recent logs. The service plist must omit `--create-db`: a missing database should fail rather than silently create an empty one.

To undo a bad binary update, boot out only `com.myles.grasshopper`, restore its saved binary and plist, and bootstrap that job. Leave the database unchanged. To disable external access, remove only the `8456` Serve mapping with the matching `tailscale serve ... off`; never use `tailscale serve reset`. If the database itself needs recovery, follow the restore procedure above before switching paths. Legacy-data conversion remains a separate, unapproved operation; see [migration and rollback](shared-memory.md).

The Mac service and Beelink HTTPS MCP read/write are verified. All three CLIs have passed synthetic cross-machine tests, but all three have **not** yet been exercised against this persistent endpoint. A reboot, Bitwarden sync from another device, and a real-profile restore are also untested. See [acceptance evidence](memory-acceptance.md).
