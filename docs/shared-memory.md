# Backend, migration, and recovery

No live Grasshopper database has been migrated or replaced. Inspect the actual host, writer, supervisor, private route, database/WAL files, model assets, credentials, and existing backups before proposing a cutover. Historical service names are not evidence of a running deployment.

## Memory contract

The authenticated HTTP MCP surface has exactly five tools: `context`, `store`, `search`, `get`, and `archive`. It has no server-filesystem indexing or code-search operation. The stateless Go client forwards MCP requests and reads canonical AGENTS.md guidance; it has no writable database.

Every write names an exact scope. `{}` explicitly means global; `project`, `device`, and `platform` can be combined. Context and search include global and matching subsets, filtering in SQLite before ranking. Full reads and mutations use the record's exact scope. Legacy unknown-scope rows never join ordinary context or search. Scope avoids accidental mixing; a bearer token still grants access to the whole personal store. Restricted data requires server-enforced boundaries or another deployment.

A confirmed preference enters context directly, independent of semantic score or decay. The bounded response contains complete records, plus omission counts and IDs. `get` can retrieve a complete current or historical revision. The agent supplies provenance and confirmation based on an explicit user decision; Grasshopper does not infer a profile from transcripts.

`store` corrects by stable ID or key and `expected_revision`. SQLite transactions reject stale concurrent corrections; the older revision remains inspectable. An unconfirmed observation cannot silently replace a confirmed preference. Similarity can suggest a record but cannot replace one. Scoped exact dedup and a durable `request_id` ledger make identical retries idempotent. A timeout is not a saved acknowledgement. Archive and restore create new revisions. Search uses scoped FTS and exact vector comparison on current rows, so a write can be found without restart or code indexing.

The client normalizes a Git origin remote into `git:host/CaseSensitivePath`, removing credentials. Project-local `grasshopper.project-id` overrides it when needed. A folder without either has unresolved project scope; it receives only applicable global/device/platform context. Device and OS data never become general project guidance.

For a new installation, `grasshopper-go-server --create-db --db /private/brain.db` creates an empty memory-only database with the required tables and indexes. It never overwrites an existing file. Supply the pinned model, runtime, token file, and loopback flags shown below; keep the database and token outside an extracted bundle. Test an authenticated read and a synthetic write, then make and reopen a consistent backup before relying on it.

## Prepare a cutover

1. Stop only the approved writer before the final snapshot. First rehearse with a synthetic or private copy. Keep the original binary, configuration, and database untouched.
2. Run `grasshopper-go-backup --source /private/brain.db --dest /private/backup.db`. It makes a consistent, no-overwrite SQLite copy including committed WAL data. Check integrity and open a restored copy; mere file existence is insufficient.
3. Run `grasshopper-go-migrate --source /private/backup.db --copy /private/shadow.db --onnx-library /private/libonnxruntime --model /private/model.onnx --tokenizer /private/tokenizer.json`. The command re-embeds memory rows in a new database, preserves IDs and history, leaves the source untouched, and removes an incomplete destination on failure. Unknown-scope legacy records stay quarantined.
4. Check IDs, legacy counts, revisions, scoped context/search, full-record reads, and semantic recall on the shadow copy. Keep the backup offline and private; it contains memories and request history.
5. Only after deployment approval, configure a loopback Go server against the verified shadow database with a private token file. Confirm unauthenticated `/healthz` is 401, authenticated health is 200, and the five-tool client path works over the approved private route.

Run `grasshopper-go-server --db /private/shadow.db --onnx-library /private/libonnxruntime --model /private/model.onnx --tokenizer /private/tokenizer.json --token-file /private/token --listen 127.0.0.1:8106` only as a task-local trial until a persistent service is approved. The server accepts only an existing fully converted database unless `--create-db` explicitly creates a new one. It checks pinned model digests and binds an explicit loopback IP. Never put a token in a CLI argument, tracked file, log, or export. Remote links require approved private HTTPS transport. The client rejects remote plaintext URLs, embedded URL credentials, redirects, and oversized responses; timeouts are bounded.

## Rollback

Before cutover, rollback is simply discarding the shadow copy. After a Go writer has accepted new writes, the pre-cutover backup no longer contains them. Stop the approved writer, preserve the Go database and WAL/SHM, and reconcile post-cutover revisions before restoring the old snapshot to a **new** path. Do not point old Rust software at a Go-written database; it can bypass revisions and its old embeddings may be incompatible. For this Go-only release, restoring a verified Go backup into a new Go service is the primary recovery path. A synthetic post-write Go backup/reopen test passed; a full live service-manager rollback has not been run.
