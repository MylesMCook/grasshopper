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
