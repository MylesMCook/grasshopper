# Mac mini operations

The private Grasshopper service is the only writer to its SQLite database. Its
launchd job is `com.myles.grasshopper`, listening on loopback port 8106 behind a
Tailnet-only Tailscale Serve route on port 8456. State is in
`~/Library/Application Support/Grasshopper`.

Daily encrypted off-host backups use `com.myles.grasshopper.backup-r2`. The
recovery secret is in the Bitwarden note **Grasshopper R2 backup recovery**.
Do not copy the secret into this repository.

## Update and recover

1. Inspect the launchd job, local listener, Tailscale Serve route, and recent
   logs. Verify a restored backup, not just a backup file. Save the current
   binary and plist. The generic snapshot command is in the [README](../README.md#back-up-and-update-the-server).
2. Stop only the Grasshopper job. Replace its binary, validate the plist, and
   start one writer against the same database and route. Check anonymous
   `/healthz` is rejected, authenticated health works, and a known memory and
   earlier revision are readable.
3. If the new binary fails, stop it and restore the saved binary and plist.
   Before restoring an older database snapshot, preserve the current DB, WAL,
   and SHM files so accepted writes can be reconciled. Restore to a separate
   path and check SQLite integrity plus record IDs and revisions.

Never reset all Tailscale Serve routes to remove one route. On 2026-09-25,
Beelink independently opened the Bitwarden recovery note, restored the latest
R2 snapshot, checked SQLite integrity, and read a known memory through a
separate loopback server. Temporary credentials and restored data were removed.
Reboot persistence and an actual host failover remain untested.

## Public site

The site is separate from the private service. Publish only after release
downloads exist. Build the ignored `web/dist` assets before each deployment:

```sh
sh web/build.sh
npx wrangler deploy --dry-run -c web/wrangler.jsonc
npx wrangler deploy -c web/wrangler.jsonc
```

Check that home, setup, and memory view load, while public `/mcp` and
`/visualizer/` return 404. To find and roll back a site deployment:

```sh
npx wrangler deployments list --name grasshopper-site
npx wrangler rollback VERSION_ID -c web/wrangler.jsonc
```

A site rollback does not change the private database or server.

## Device pairing upgrade

Version 2.3.0 adds a `client_tokens` table to the existing SQLite database.
It stores token hashes and revocation status, not raw device tokens. Rehearse
the additive upgrade on a restored copy and verify a current backup before a
live restart. The 2.2.5 binary ignores the extra table on rollback; new device
tokens then stop working until the newer server returns. The master token and
memory records remain intact.
