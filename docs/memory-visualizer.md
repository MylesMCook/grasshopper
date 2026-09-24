# Live memory view

The server bundle includes a read-only visualizer. It shows confirmed memories and the latest handoff for the scope you choose. It refreshes every three seconds while connected. A correction or new confirmed memory appears without reloading the page.

## Start

1. Start `grasshopper-go-server` with its normal database, model, token, and loopback flags. Add `--visualizer` to enable the view.
2. Open `http://127.0.0.1:8106/visualizer/` on the server host. Use your actual loopback port if different.
3. Paste the existing bearer token into the page. Add a durable project ID, device, or platform to see applicable scoped records. Blank fields show global context only.

The token stays in this tab's memory. The page sends it in an Authorization header; it never puts it in a URL, cookie, or browser storage. Disconnect or close the tab to clear it. The blank page shell is public, but every memory read requires the bearer token. Do not use an untrusted browser or screen share while viewing private memories. For access from another machine, configure an approved private HTTPS route and keep Authorization headers out of proxy logs. The server itself binds to loopback.

The page shows full returned records with scope, revision, confirmation, and source. If the response budget omits records, it says how many. Use the MCP `get` tool for a complete omitted record or a prior revision. Archived, legacy, and unconfirmed observation records are not in this context view. The visualizer cannot write, archive, or index files.

Scope keeps records from mixing; it is not an access boundary. Anyone with the service token can request any scope. Use separate deployments when readers need different access rights.

If the service stops or the read fails, the page marks its last result stale and stops polling. Reconnect after the service returns. A rejected token clears the displayed records.

## Remove

Stop the server and restart it without `--visualizer`. No memory data changes and no separate visualizer storage exists. The route then returns 404 to authenticated requests.
