# Public website

`usegrasshopper.com` explains Grasshopper, links to the release downloads, and lets visitors open their own server's memory view. `/view/` remembers only the server address in the browser. Tokens and memories stay on that server; this site does not proxy them.

## Build and deploy

From the repository root:

```sh
sh web/build.sh
wrangler deploy --dry-run -c web/wrangler.jsonc
wrangler deploy -c web/wrangler.jsonc
```

`web/dist/` is generated from `web/public/`, the shared stylesheet and theme script, and the [licensed fonts](../docs/fonts/README.md). It is ignored by Git. Check the Cloudflare account, domain, preview, and release download links before deploying. The Worker serves the `usegrasshopper.com` custom domain.

## Check and roll back

Open `/`, `/setup/`, and `/view/` in a browser. Check links, theme, narrow-screen layout, and console errors. Public `/mcp` and `/visualizer/` must return 404. To roll back only the website, find the prior version with `wrangler deployments list --name grasshopper-site`, then run `wrangler rollback VERSION_ID -c web/wrangler.jsonc`. A site rollback does not change a private memory server.

The `no-transform` response header prevents Cloudflare from injecting a script blocked by this site's content security policy. Two exact style hashes allow Codex Annotate in the tested app build; keep `unsafe-inline` disabled. If annotation breaks after a Codex update, inspect the blocked styles before changing those hashes. `web/build.sh` changes each page asset when `_headers` changes so a header-only deploy reaches the edge.
