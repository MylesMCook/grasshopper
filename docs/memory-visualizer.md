# Live memory view

The server bundle includes a read-only visualizer. It shows confirmed memories and the latest handoff for the scope you choose. It refreshes every three seconds while connected. A correction or new confirmed memory appears without reloading the page.

## Start

1. Start `grasshopper-go-server` with its normal database, model, token, and loopback flags. Add `--visualizer` to enable the view.
2. Open `http://127.0.0.1:8106/visualizer/` on the server host. Use your actual loopback port if different.
3. Paste the existing bearer token once. The default shows global memories across devices and platforms. Choose a known project to add its records. Choose a device or platform to narrow the view.

The page exchanges the token for a signed browser cookie valid for 30 days. The cookie is HttpOnly, SameSite=Strict, and limited to visualizer API paths; private HTTPS connections also set Secure. It cannot authorize MCP tools or health checks. The token is not kept in local storage, session storage, a URL, or the cookie. **Disconnect** clears this browser's cookie. Closing the tab does not. Rotating the server token invalidates all browser sessions; Disconnect does not revoke a copied cookie elsewhere. Use a trusted browser and a private HTTPS route when viewing from another machine. The server still binds to loopback.

The memory view's **How it works** link opens the [public Grasshopper site](https://usegrasshopper.com/). The private server serves no setup or explanation page. The theme switch stores only a display preference in this browser; it never stores the access token.

The page shows each full returned memory. Open **Details** to inspect its scope, revision, confirmation, and source. Live refresh leaves unchanged text in place; if you select a memory's text, a changed result waits until you release the selection. Project choices come from active project records; device choices come from active records applicable to the selected project and platform. If the response budget omits records, it says how many. Use the MCP `get` tool for a complete omitted record or a prior revision. Archived, legacy, and unconfirmed observation records are not in this view. The visualizer cannot write, archive, or index files.

**Any device** is an inspection view across machines. An agent's own `context` call remains narrower: it loads only records applicable to that agent's project, device, and platform. Do not treat a Windows-specific record in the visualizer as a Mac instruction.

Scope keeps records from mixing; it is not an access boundary. Anyone with the service token can request any scope. Use separate deployments when readers need different access rights.

If the service stops or the read fails, the page marks its last result stale and stops polling. Reconnect after the service returns. A rejected token clears the displayed records.

Codex's in-app annotation mode may inject styles that the default strict CSP blocks. If that failure is confirmed for a specific Codex build, the server accepts an optional `--visualizer-style-hashes` list of exact, comma-separated SHA-256 hashes. This adds only those style sources to the visualizer document. Leave the flag unset for the default policy; recompute and retest the hashes after a Codex update. Do not use `unsafe-inline`.

## Remove

Stop the server and restart it without `--visualizer`. No memory data changes and no separate visualizer storage exists. The route then returns 404 to authenticated requests.
