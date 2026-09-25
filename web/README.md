# Public Grasshopper site

`usegrasshopper.com` serves two static pages: a short introduction and a memory-view
link. Visitors enter their own server address at `/view/`; the page remembers only
that address in the browser, then opens the server's `/visualizer/`. It does not
accept tokens, proxy requests, or hold memories. Visitors enter their token on
their own server.

Build with `sh web/build.sh`, then run `wrangler deploy --dry-run -c web/wrangler.jsonc`
from the repository root. The deployment source is `web/dist/`, generated from
`web/public/` plus the repository's site stylesheet, theme script, and fonts.
The directory is ignored by Git. Deploy with `wrangler deploy -c web/wrangler.jsonc`
only after checking the account, domain, and preview behavior. The config attaches
the Worker to the `usegrasshopper.com` custom domain. The response header
`Cache-Control: public, max-age=0, no-transform` prevents Cloudflare from injecting
its analytics script into a page whose CSP allows only first-party scripts.

After deployment, check `/` and `/view/` in a browser, and confirm that `/mcp`
and `/visualizer/` return 404 on the public domain. For a site rollback, find the
previous version with `wrangler deployments list --name grasshopper-site`, then
run `wrangler rollback VERSION_ID -c web/wrangler.jsonc`. This changes only the
public site; it does not touch a private memory server.

The repository is private, so the public site labels installation as a private
pilot. Do not add links to its private GitHub releases until those files are
actually public.

The public pages allow two exact style hashes used by Codex Annotate in the
tested app build. Keep `unsafe-inline` disabled. If annotation breaks after a
Codex update, inspect the new blocked styles before changing the hashes.
`web/build.sh` changes each page asset when `_headers` changes so a header-only
deployment cannot leave the previous policy at the edge.
