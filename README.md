# Grasshopper

Grasshopper keeps your explicit preferences, project decisions, verified lessons, and short handoffs in one private memory service. Codex, Cursor, and Claude Code can read the same records across machines.

[How it works](https://usegrasshopper.com/) · [Set up a server](https://usegrasshopper.com/setup/) · [Install for Codex or Cursor](integrations/plugins/marketplace-README.md) · [Connect Claude Code](integrations/plugins/README.md) · [What we tested](docs/memory-acceptance.md)

## Start

1. Run the [server](https://usegrasshopper.com/setup/) on a machine you control. It owns the only writable memory database.
2. [Install the Codex or Cursor connector](integrations/plugins/marketplace-README.md) on each agent machine. [Claude Code uses the client package](integrations/plugins/README.md). Each connects to that same server with a private token file.
3. Save one explicit preference. In a fresh agent session, check that it arrived before tool use; inspect it in the server's [memory view](docs/memory-visualizer.md).

Memories have scope, source, and revision history. Project scope keeps records from mixing, but it does not restrict access. Use a separate service for data that needs a separate access boundary.

For repository work, read [AGENTS.md](AGENTS.md). For backups, migration, and recovery, use the [operator guide](docs/shared-memory.md). The [shared AGENTS.md policy](integrations/policy/AGENTS.md) is the one standing memory instruction source for all three agents.
