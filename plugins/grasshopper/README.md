# Grasshopper

Install this plugin from Grasshopper's own marketplace to connect Codex to your
Grasshopper service. It adds the MCP connection and startup hooks. The client,
server, token, and shared AGENTS.md policy are separate. The plugin keeps no
memory database or copy of your instructions.

## Set up

1. [Download the Grasshopper client for your machine](https://github.com/MylesMCook/grasshopper/releases/latest) and put its `grasshopper` binary on the PATH used by Codex Desktop and CLI. On Windows the file is `grasshopper.exe`.
2. Follow the [client connection guide](https://github.com/MylesMCook/grasshopper/blob/main/integrations/plugins/README.md) to create `client.json`, a private token file, and the single shared `AGENTS.md` policy. The client must point to your own authenticated server.
3. Add the Grasshopper repository as its own marketplace, then install the plugin:

   `codex plugin marketplace add MylesMCook/grasshopper --ref main`

   `codex plugin add grasshopper@grasshopper-marketplace`

4. In a new Codex task, review and trust the hooks in `/hooks`. Ask for Grasshopper context and confirm a known synthetic record before using real memory.

The plugin does not start a server or copy credentials. This Git-backed
marketplace currently requires access to the Grasshopper repository. Codex Desktop may have a
different PATH from a terminal; check that its MCP server starts and its hooks
load in a fresh task. If startup memory is absent, the canonical policy asks
for one MCP `context` call. A failed write is not a saved memory.

If you already use the archive's `grasshopper@grasshopper-local` plugin, remove
that installation before enabling this one so two Grasshopper MCP sources and
hooks do not load together.

## Remove

Run `codex plugin remove grasshopper@grasshopper-marketplace`. Keep or remove the
client, config, token, and shared policy separately, depending on whether Cursor
or Claude Code still uses them. Removing this plugin does not delete memories.
