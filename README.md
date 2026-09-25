# Grasshopper marketplace

**One server, many agents.** Your private server owns the memories. Install a
small connector on each machine; connectors keep no memory database or token.

[What passed and what did not](https://github.com/MylesMCook/grasshopper/blob/main/docs/memory-acceptance.md#marketplace-pilot-25-september).

## Install

1. [Start a server](https://usegrasshopper.com/setup/) and keep its HTTPS
   `/mcp` address and token file ready.
2. Add this marketplace. Choose `grasshopper-macos` for Apple Silicon,
   `grasshopper-windows` for Windows x64, or `grasshopper-linux` for Linux x64.

   **Codex**

   ```sh
   codex plugin marketplace add MylesMCook/grasshopper --ref marketplace
   codex plugin add grasshopper-macos@grasshopper-marketplace
   ```

   **Cursor Agent CLI**

   ```sh
   agent plugin marketplace add https://github.com/MylesMCook/grasshopper.git --git-ref marketplace
   ```

   In Cursor Agent CLI, run `agent`, then `/plugins` and install your OS entry.
   In the IDE, use **Customize → Plugins**. The CLI has no noninteractive
   plugin-install command.
3. Ask the agent: **Use the connect-grasshopper skill.** Give it the server
   address and the *path* to a token file on this machine. Do not paste the
   token into chat. For Cursor Agent CLI, the skill adds its MCP entry to your
   user-level Cursor config. Keep that default on Windows.
4. In Codex, review and trust Grasshopper's hooks with `/hooks`. In Cursor
   Agent CLI, run `agent mcp enable grasshopper`. Start a fresh session and ask
   which known memory arrived before tools. If none did, call `context` once.

## Update or remove

For Codex, run `codex plugin marketplace upgrade grasshopper-marketplace`,
then `codex plugin add` for your OS entry. For Cursor Agent CLI, open
`/plugin list` and uninstall Grasshopper, then run:

```sh
agent plugin marketplace remove grasshopper-marketplace
agent plugin marketplace add https://github.com/MylesMCook/grasshopper.git --git-ref marketplace
```

Reinstall your OS entry in `/plugin list` → Marketplace. Cursor Agent CLI
`marketplace update` can report success while retaining the old Git snapshot;
the [Cursor-supported workaround](https://forum.cursor.com/t/cli-added-git-marketplace-never-reindexes-plugin-marketplace-update-reports-0-plugins-indexed-and-cache-stays-pinned-to-the-first-commit/169587/7)
is remove and re-add. After either harness updates, run the connect skill
with `--update`, then `check` and test a fresh session. Keep the old package
until the new one passes.

Remove the plugin in its harness. Removing a connector does not delete server
memories. Keep the shared config and token file if another agent uses them.

For a manual recovery path, see the [client guide](https://github.com/MylesMCook/grasshopper/blob/main/integrations/plugins/README.md).
