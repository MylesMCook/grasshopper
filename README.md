# Grasshopper marketplace

**One server, many agents.** Your private server owns the memories. Install a
small connector on each machine; connectors keep no memory database or token.

This is a pilot from the `marketplace-candidate` branch. The release branch is
not published yet. [What passed and what did not](https://github.com/MylesMCook/grasshopper/blob/main/docs/memory-acceptance.md#marketplace-pilot-25-september).

## Install

1. [Start a server](https://usegrasshopper.com/setup/) and keep its HTTPS
   `/mcp` address and token file ready.
2. Add this marketplace. Choose `grasshopper-macos` for Apple Silicon,
   `grasshopper-windows` for Windows x64, or `grasshopper-linux` for Linux x64.

   **Codex**

   ```sh
   codex plugin marketplace add MylesMCook/grasshopper --ref marketplace-candidate
   codex plugin add grasshopper-macos@grasshopper-marketplace
   ```

   **Cursor Agent CLI**

   ```sh
   agent plugin marketplace add https://github.com/MylesMCook/grasshopper.git --git-ref marketplace-candidate
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
then reinstall your OS entry with `codex plugin add`. For Cursor, run
`agent plugin marketplace update grasshopper-marketplace`, then update your
entry in `/plugins` or Customize. Run the connect skill with `--update`,
then `check` and test a fresh session. A changed-version marketplace update
still needs a real-client rehearsal.

Remove the plugin in its harness. Removing a connector does not delete server
memories. Keep the shared config and token file if another agent uses them.

For a manual recovery path, see the [client guide](https://github.com/MylesMCook/grasshopper/blob/main/integrations/plugins/README.md).
