# Grasshopper

Grasshopper remembers explicit preferences, project decisions, verified lessons, and short handoffs for Codex, Cursor, and Claude Code. One private server holds the records; each agent connects to it.

[How it works](https://usegrasshopper.com/) · [Set up](SETUP.md) · [Open a memory view](https://usegrasshopper.com/view/) · [What passed](docs/memory-acceptance.md)

Start an empty server, connect an agent, and ask it to save one preference you actually want kept. In a fresh session, ask what it received before tool use. Each record has a scope, source, and revision history. Project scope prevents accidental mixing; it is not an access boundary.

For repository work, read [AGENTS.md](AGENTS.md). The [shared memory policy](integrations/policy/AGENTS.md) is the only standing memory-use policy. Operators can use the [backup and recovery guide](docs/shared-memory.md).
