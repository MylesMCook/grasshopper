# Self-hosted server operations

Grasshopper has one authenticated server and one authoritative SQLite database.
Keep deployment addresses, supervisor names, credential locations and recovery
secrets in your private operator notes, outside this public repository.

## Update and recover

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

## Public site

The site is separate from the private server and contains no memory data.
Publish only after release downloads exist. The repository's existing site
configuration defines the deployment target:

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
