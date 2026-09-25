# Grasshopper

Grasshopper keeps a few useful memories in one private place. Codex, Cursor,
and Claude Code can share your explicit preferences, project decisions, verified
lessons, and short handoffs across machines.

[See how it works](docs/how-it-works.html) · [Download the latest release](https://github.com/MylesMCook/grasshopper/releases/latest) · [See what has been tested](docs/memory-acceptance.md)

## Start here

1. [Run the server](docs/go-package.md) on a host you control. It holds the only writable memory database.
2. [Connect your agents](integrations/plugins/README.md) to that same service. Codex users can install the plugin from [Grasshopper's own marketplace](plugins/grasshopper/README.md).
3. Save one explicit preference, then check it in a fresh session and in the [memory view](docs/memory-visualizer.md).

The server requires authentication. Memories can be corrected by revision and
archived without losing their history. Project scope keeps records from mixing,
but it is not an access boundary. Keep restricted work in a separate deployment.

For development, see [repository instructions](AGENTS.md). For data handling and
recovery, see [the operator guide](docs/shared-memory.md). The
[shared memory policy](integrations/policy/AGENTS.md) is the only standing
instruction file maintained for all three agents.
