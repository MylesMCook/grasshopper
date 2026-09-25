# Operate Grasshopper

One authenticated SQLite server owns all writable memories. Clients have no memory database. The MCP surface is `context`, `store`, `search`, `get`, and `archive`; it cannot index server files. A token grants access to the whole store. Project scope prevents accidental mixing, not unauthorized access.

## Back up and update

Use the [release archive](https://github.com/MylesMCook/grasshopper/releases/latest) for your server OS. Check `SHA256SUMS` before using it. The server archive includes the pinned model, tokenizer, runtime, binaries, licenses, [setup guide](../SETUP.md), and this runbook; it includes no token or database. Build maintainers can use `scripts/build-client-release.sh VERSION OUTPUT_DIR` for client archives and `go run ./cmd/grasshopper-go-bundle --help` for server packaging.

For a quickstart server, run the **new** archive's `grasshopper-backup --quickstart` before stopping the old process. Add `--data-dir ABSOLUTE_PATH` if you used one. The command makes a consistent SQLite snapshot and checks it. Keep an encrypted off-host copy. Save the old binary and service settings, then start the new binary with the **same** state directory, listener, and private route. Check anonymous `/healthz` returns 401, authenticated health returns 200, a full-record read works, and logs show no startup error. Keep the old binary until these pass.

If the new binary fails, stop it and restart the old binary against the preserved database. Never launch two writers. A pre-update backup omits any accepted writes after it was taken; preserve current DB/WAL/SHM before restoring an older snapshot. Restore to a **new** path, check SQLite integrity and record IDs/revisions, then point one server at it. Do not claim recovery based on a backup file's existence.

For a manually configured database, `grasshopper-backup --source DB --dest NEW_BACKUP` makes a no-overwrite copy, including committed WAL data. Keep the model/tokenizer/runtime and token outside the extracted archive. Bind the server to loopback. A remote client needs private HTTPS; `--allowed-proxy-host` must be the exact HTTPS host and port, never a wildcard. Do not expose the memory endpoint publicly.

The current Mac mini deployment uses launchd job `com.myles.grasshopper` on `127.0.0.1:8106` and a Tailnet-only Serve route on port `8456`. State is under `~/Library/Application Support/Grasshopper`. Daily encrypted R2 backups use `com.myles.grasshopper.backup-r2`; the recovery secret is in the Bitwarden note **Grasshopper R2 backup recovery**. Before changing this service, check its launchd status, listener, private route, recent logs, and a restored backup. Preserve its binary and plist for rollback. Never use `tailscale serve reset` to remove one route. Reboot and full Mac-loss recovery remain untested.

## Migrate an older database

Legacy records with uncertain scope stay quarantined from ordinary context. Rehearse on a private copy first: back up the old database, run `grasshopper-migrate --source BACKUP --copy SHADOW --onnx-library LIB --model MODEL --tokenizer TOKENIZER`, and verify IDs, history, scope, semantic recall, and SQLite integrity on `SHADOW`. Migration never edits its source. Only after deployment approval, stop the old writer and cut over to one server. Roll back to the preserved copy only after reconciling accepted post-cutover writes. Old Rust software must not write the new database.

## Public site

`usegrasshopper.com` serves explanations, downloads, and a launcher for a user's own memory view. It does not proxy tokens or memories. From the repository root: `sh web/build.sh`, `wrangler deploy --dry-run -c web/wrangler.jsonc`, then `wrangler deploy -c web/wrangler.jsonc`. Verify `/`, `/setup/`, and `/view/` in a browser; public `/mcp` and `/visualizer/` must return 404. Roll back only the site with `wrangler deployments list --name grasshopper-site` and `wrangler rollback VERSION_ID -c web/wrangler.jsonc`. A site rollback does not alter the private service.

Newsreader and Geist Mono are bundled under the OFL texts in `docs/fonts/`. The public site's two exact annotation style hashes are intentional; check blocked styles after a Codex update rather than broadening CSP.
