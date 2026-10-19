# Bugbot review guidance

Grasshopper is one authenticated, self-hosted SQLite memory service with a
stateless client. Read `AGENTS.md` for the full rules. Report confirmed defects
with their observable impact; skip style preferences.

Check especially:

- Scope: global, project, device and platform records never mix. A missing or
  folder-name project is unresolved, never global. New project writes need
  `id:` or `git:host/path` identity.
- Writes: expected revisions, request-ID idempotency (a retry with the same ID
  replays; a different payload conflicts), revision history, and reversible
  archive. New writes must be searchable immediately.
- Owner endpoints: loopback or the configured private proxy only, Origin
  checks, HttpOnly session cookie. Stored memory text is placed only with
  `textContent`.
- Errors: validation failures return 400 and conflicts 409 with the current
  record; only real server faults return 5xx, because clients retry those.
- Secrets, tokens, private hostnames and machine names never appear in code,
  logs, docs or tests.
