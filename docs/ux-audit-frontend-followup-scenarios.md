# Viewer follow-up acceptance scenarios

The existing Node viewer harness uses synthetic data and asserts the following accepted behaviors in `internal/gomcp/visualizer/app.test.cjs`:

- A persisted light or dark theme applies with no body DOM, before the stylesheet.
- Kind survives a URL reload and scopes both browsing and search.
- Show more uses the server cursor; changed polling rebuilds the loaded range from a new snapshot rather than retaining stale tail records.
- A scope change aborts a pending page and ignores its response. A 304 preserves displayed content and status.
- Confirm sends only its action, ID, expected revision, and retry ID, preserving source fields on the server.
- Owner-edited detail loads revision one to name the original harness and device.
- Archive offers Undo against the acknowledged archive revision; an uncertain retry reuses its request ID.
- Sign out everywhere accepts 204 and invalidates pending viewer responses.
- Export sends the selected scope, explicit all-memories choice, and archived choice.

Existing assertions for initial eight records, inline review, dirty drafts, revision restore, search emphasis, polling visibility/backoff, and byte limits remain intact.

Integrated review regressions add these cases:

- After a failed poll, an unchanged successful response restores Live and the correct browse/startup count without replacing memory content or duplicating timers.
- A snapshot redraw deferred while selecting text occurs on the next unchanged poll after selection ends.
- Show more pressed during polling waits and uses the new snapshot cursor.
- A malformed 200 export fails without creating a download; a valid records export succeeds.
- Browse counts show the server total beyond the first byte-limited page; nonessential management controls follow the memory list in Devices & connections.
