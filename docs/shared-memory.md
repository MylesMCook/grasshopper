# Operate Grasshopper

One authenticated SQLite server owns all writable memories. Clients have no memory database. The MCP surface has five tools: context, store, search, get, and archive. It cannot index server files. A token grants access to the whole store; project scope is not access control.

## Back up and update

Use the [server release archive](https://github.com/MylesMCook/grasshopper/releases/latest) for your OS. Check its SHA256SUMS file. It includes the pinned model, tokenizer, runtime, binaries, licenses, [README](../README.md), and this runbook; no token or database. On Windows, use the matching .exe commands in PowerShell.

Before replacing a quickstart server, run the backup tool from the **new** archive:

```sh
./bin/grasshopper-backup --quickstart
```

Add your existing data directory if you used one:

```sh
./bin/grasshopper-backup --quickstart --data-dir ABSOLUTE_PATH
```

The backup tool makes and checks a consistent SQLite snapshot. Keep an encrypted off-host copy. Save the old binary and service settings. Start the new binary with the **same** state directory, listener, and private route. Confirm anonymous health returns 401, authenticated health returns 200, a full-record read works, and logs show no startup error. Keep the old binary until these pass.

If the new binary fails, stop it and restart the old binary against the preserved database. Never run two writers. A pre-update backup omits accepted writes after it was taken; preserve current DB/WAL/SHM before restoring an older snapshot. Restore to a **new** path, check SQLite integrity and record IDs/revisions, then point one server at it. Backup-file existence alone is not a recovery test.

For a manually configured database, make a no-overwrite copy, including committed WAL data:

```sh
./bin/grasshopper-backup --source DB --dest NEW_BACKUP
```

Keep the model, tokenizer, runtime, and token outside the extracted archive. Bind the server to loopback. Remote clients need private HTTPS. The allowed proxy host must be the exact HTTPS host and port, never a wildcard. Do not expose the memory endpoint publicly.

The Mac mini deployment uses launchd job com.myles.grasshopper on loopback port 8106 and a Tailnet-only Serve route on port 8456. State is under ~/Library/Application Support/Grasshopper. Daily encrypted R2 backups use com.myles.grasshopper.backup-r2; the recovery secret is in the Bitwarden note **Grasshopper R2 backup recovery**. Before changing this service, check launchd, the listener, route, recent logs, and a restored backup. Preserve its binary and plist. Never reset all Tailscale Serve routes to remove one route. Reboot and full Mac-loss recovery remain untested.

## Migrate an older database

Legacy records with uncertain scope stay quarantined from ordinary context. Rehearse on a private copy: back up the old database, then run:

```sh
./bin/grasshopper-migrate --source BACKUP --copy SHADOW --onnx-library LIB --model MODEL --tokenizer TOKENIZER
```

Verify IDs, history, scope, semantic recall, and SQLite integrity on the shadow copy. Migration never edits its source. Only after deployment approval, stop the old writer and cut over to one server. Roll back to the preserved copy only after reconciling accepted post-cutover writes. Old Rust software must not write the new database.

## Public site

[usegrasshopper.com](https://usegrasshopper.com/) serves explanations, downloads, and a launcher for a user's own memory view. It does not proxy tokens or memories. From the repository root:

```sh
sh web/build.sh
npx wrangler deploy --dry-run -c web/wrangler.jsonc
npx wrangler deploy -c web/wrangler.jsonc
```

Verify the home, setup, and memory-view pages in a browser; public MCP and private-view routes must return 404. Roll back only the site with:

```sh
npx npx wrangler deployments list --name grasshopper-site
npx wrangler rollback VERSION_ID -c web/wrangler.jsonc
```

A site rollback does not alter the private service. Newsreader and Geist Mono are bundled under the OFL texts in docs/fonts. The site's two exact annotation style hashes are intentional; check blocked styles after a Codex update rather than broadening CSP.
