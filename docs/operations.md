# Self-hosted server operations

Grasshopper has one authenticated server and one authoritative SQLite database.
Keep deployment addresses, supervisor names, credential locations and recovery
secrets in your private operator notes, outside this public repository.

## Update and recover

### One command on macOS

For a server run by launchd from an extracted archive, run this on the server
machine, from the current archive or a newer one:

```sh
./bin/grasshopper-server upgrade
```

It finds the one plist in `~/Library/LaunchAgents` that runs
`grasshopper-server` (pass `--plist` if there are several). It downloads the
latest release (or `--to X.Y.Z`), checks it against
`SHA256SUMS`, and extracts it beside the current archive as
`grasshopper-server-X.Y.Z`. It backs up and verifies the database into
`backups` next to it, points the `current` link beside the archives at the new
one and has launchd restart the job, then checks authenticated health. If the
new server does not report the new version, `current` goes back and the old
server restarts. launchd runs at most one process per job, so two servers never
share the database. The database, token, listener and proxy flags are kept as
they are.

The first upgrade moves the plist onto `current` once (the old plist is saved
as `.before-current`) and restarts the same version before switching.
The `previous` link keeps the replaced archive. `upgrade --rollback` switches
back; running it again goes forward. Older extracted archives are removed.
To upgrade automatically, run `./bin/grasshopper-server upgrade --schedule`
once. It installs a second launchd job beside the service plist that runs
`upgrade` at 03:30 each night with the server's current program, logging to
`~/Library/Logs/Grasshopper/upgrade.log`. A failed night leaves the previous
server running. `--unschedule` removes it.

A service set up by hand, whose plist runs a renamed binary or a folder that is
not a release archive, is refused. Extract the release archive of the running
version, point the plist's program, runtime and model paths into it, restart,
then upgrade. Servers before 2.11.0 cannot run `upgrade`; run it from the newer
archive's `bin/grasshopper-server`.

Clients follow the server's version on their own, so a rollback also rolls them
back. Keep an off-machine backup as well.

### By hand

1. Inspect the configured supervisor, listener, private HTTPS route, data paths,
   dependencies and recent logs. Run the [backup tool](../README.md#back-up-and-update-the-server)
   and verify a restored copy. Keep the current archive and configuration.
2. Rehearse any required migration on a copy. Stop only the Grasshopper service
   before cutover, and run only one writer. Retain the credentials and private
   routing. Validate the new configuration before starting the server.
3. Check that anonymous API requests are rejected, authenticated health works,
   and a known memory and earlier revision are readable. Verify the browser
   and an installed agent can reach the private server.
4. If verification fails, stop the new server and restore the saved archive and
   configuration. Preserve any database, WAL and SHM files that accepted new
   writes. Restore a backup to a separate path and check SQLite integrity,
   record IDs and revisions before cutover; reconcile accepted writes first.

Do not infer restart or machine-loss recovery from configuration inspection.
Test recovery separately with copied data. Change only the relevant private
route, preserving unrelated services and remote access.

### Back up a custom database

If you chose a custom quickstart folder, add `--data-dir` with its absolute
path. For a server started with `--db`, write the snapshot to a new path:

```sh
./bin/grasshopper-backup --source /absolute/path/to/memory.db --dest /absolute/path/to/new-backup.db
```

Backup and `--verify` both print the record count, revision count, SQLite
`quick_check`, size and SHA-256. Verification is read-only and fails on a
corrupt, empty or non-private file, or one with pending WAL or journal state.
Compare the digest after copying a backup off the machine.

### Restore a backup

With the server stopped, copy a verified snapshot to a new private path (mode
0600 on macOS and Linux), verify the copy, and start the server against it with
the same token, listener and private route settings. For `--quickstart`, keep
the old state folder and replace its `memory.db` with the verified copy. Open a
known memory and an earlier revision before relying on it. Keep the original
database and any WAL/SHM files, and reconcile writes it accepted before rolling
back.

### Re-embed when upgrading from 2.3.x or earlier

2.4.0 switched to the [Granite model](embedding-model.md), which uses a
different vector space. From the new server archive, rehearse on a backup first,
then run again with the old server stopped and a destination that does not exist:

```sh
./bin/grasshopper-migrate --source /absolute/path/to/memory.db \
  --copy /absolute/path/to/memory-granite.db \
  --onnx-library ./runtime/libonnxruntime.dylib \
  --model ./models/granite-embedding-small-english-r2/model.onnx \
  --tokenizer ./models/granite-embedding-small-english-r2/tokenizer.json
```

On Linux use `./runtime/libonnxruntime.so`; on Windows use
`grasshopper-migrate.exe`, `runtime/onnxruntime.dll` and Windows paths on one
line. Keep `model.onnx_data` next to `model.onnx`. Start the new server against
the verified copy as in [Restore a backup](#restore-a-backup).

## Unsigned archives

Release executables are unsigned. After checking the archive against
`SHA256SUMS`, macOS may require **System Settings > Privacy & Security > Open
Anyway** ([Apple's instructions](https://support.apple.com/en-us/102445)). Do not
disable Gatekeeper or remove quarantine recursively. On Windows, follow
[SmartScreen guidance](https://learn.microsoft.com/en-us/windows/security/operating-system-security/virus-and-threat-protection/microsoft-defender-smartscreen/)
if blocked; this has not been tested. Linux archives keep executable permissions.

## Build from source

Source builds compile the binaries only; they do not fetch the ONNX runtime,
model or tokenizer that `--quickstart` expects from an archive.

```sh
mkdir -p bin
go build -o ./bin/grasshopper ./cmd/grasshopper
go build -o ./bin/grasshopper-server ./cmd/grasshopper-server
go build -o ./bin/grasshopper-backup ./cmd/grasshopper-backup
go build -o ./bin/grasshopper-migrate ./cmd/grasshopper-migrate
```

On Windows, add `.exe` to each output. A source-built server needs `--db`,
`--token-file`, `--onnx-library`, `--model` and `--tokenizer`; add `--create-db`
for an empty store and `--visualizer` for the memory view. Each binary has
`--help`.

## Public site

The site is separate from the private server and contains no memory data.
The release workflow deploys it after each release. To deploy a site-only change
by hand, after the current release downloads exist:

```sh
sh web/build.sh
npx wrangler deploy --dry-run -c web/wrangler.jsonc
npx wrangler deploy -c web/wrangler.jsonc
```

Verify home, setup and memory-view guidance, while public `/mcp` and
`/visualizer/` return 404. Retain the deployment version for rollback through
Wrangler. A site rollback does not change the private database or server.

## Device pairing

Pairing stores device token hashes and revocation status in SQLite, never raw
credentials. Rehearse schema upgrades on a restored copy before restarting the
live server. An older binary may not accept newly paired devices; retain its
archive, configuration and a verified recovery copy until the update is checked.

## Keep the server running

First run the extracted archive in a terminal and verify the memory view. These
are optional templates, not an installer. Use absolute paths, keep one writer,
and retain the previous configuration. Keep the loopback listener; an existing
private HTTPS proxy needs `--allowed-proxy-host hostname:443` on every start.

### macOS launchd

The archive includes `packaging/macos/com.example.grasshopper.plist`. Replace
`/Users/you` paths with your own archive, data and token paths. Create its logs
folder first with private permissions, for example:

```sh
mkdir -p "$HOME/Library/Application Support/Grasshopper/logs"
chmod 700 "$HOME/Library/Application Support/Grasshopper/logs"
plutil -lint /absolute/path/com.example.grasshopper.plist
launchctl bootstrap "gui/$(id -u)" /absolute/path/com.example.grasshopper.plist
```

Stop that job before replacement or token rotation:

```sh
launchctl bootout "gui/$(id -u)" /absolute/path/com.example.grasshopper.plist
```

The template uses the archive's `bin/grasshopper-server` and includes a commented
private proxy option. Inspect logs and the memory view after starting it.

### Linux systemd user unit

Save this as `~/.config/systemd/user/grasshopper.service`, substituting absolute
paths. The data directory must be private.

```ini
[Unit]
Description=Grasshopper memory server

[Service]
ExecStart=/absolute/archive/bin/grasshopper-server --quickstart --data-dir /absolute/private/Grasshopper
Restart=on-failure
RestartSec=30
UMask=0077

[Install]
WantedBy=default.target
```

Use `systemctl --user daemon-reload` and `systemctl --user enable --now
grasshopper.service`. Check `systemctl --user status grasshopper.service` and
`journalctl --user -u grasshopper.service`. Stop it with `systemctl --user stop
grasshopper.service` before replacement. User-session startup depends on your
existing systemd login configuration; this does not enable linger.

### Windows Task Scheduler

Create a task for your own account triggered **At log on**. Set **Program/script**
to the archive's absolute `bin\grasshopper-server.exe`, **Add arguments** to
`--quickstart --data-dir "C:\absolute\private\Grasshopper"`, and **Start in** to
the extracted archive folder. Choose **Do not start a new instance** if already
running. Keep the folder accessible only to your account. End this task before
replacement or rotation, then run it again and check Task Scheduler status and
the memory view. This template was inspected, not installed or reboot-tested.

## Recover or rotate the owner token

Stop every server using the state folder first. For quickstart state, run:

```sh
./bin/grasshopper-server --rotate-token --server-stopped --data-dir /absolute/private/Grasshopper
```

On Windows use `bin\grasshopper-server.exe`. The command requires the existing
database, atomically replaces the private `access-token` file, and retains the
old credential in `access-token.previous` if present. It also recovers a lost
token file. It does not change the database or paired-device credentials, stop a
process, or modify a supervisor. `--server-stopped` acknowledges that you already
stopped all writers; it is not a process detector.

Restart with the same data directory, then sign in using the new token file.
Old owner credentials and cookies stop working only after the server reloads
the new token. A running server retains its old credential. Revoke individual
paired devices separately if needed. If startup fails, stop the server and copy
`access-token.previous` over `access-token`, keeping it private, then restart.
After verification, delete the rollback credential if the previous token leaked.
The command refuses to overwrite an existing rollback file.
