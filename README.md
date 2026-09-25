# Grasshopper marketplace

**One server, many agents.** Your private server owns the memories. Install a
small connector on each machine; connectors keep no memory database or token.

This marketplace is being tested. The `marketplace` branch is not published yet.

## Install

1. [Start a server](https://usegrasshopper.com/setup/) and keep its HTTPS
   `/mcp` address and token file ready.
2. Add this marketplace. On macOS, install `grasshopper-macos`; on Windows or
   Linux, choose `grasshopper-windows` or `grasshopper-linux`.

   **Codex**

   ```sh
   codex plugin marketplace add MylesMCook/grasshopper --ref marketplace
   codex plugin add grasshopper-macos@grasshopper-marketplace
   ```

   **Cursor Agent CLI**

   ```sh
   agent plugin marketplace add https://github.com/MylesMCook/grasshopper.git --git-ref marketplace
   ```

   In Cursor, use **Customize → Plugins** to install your OS entry. The CLI
   does not yet have a plugin-install command.
3. Ask the agent: **Use the connect-grasshopper skill.** Give it the server
   address and the *path* to a token file on this machine. Do not paste the
   token into chat. For Cursor Agent CLI, the skill adds its MCP entry.
4. Approve Codex's startup hooks in `/hooks` or Cursor's MCP prompt. Start a
   fresh session and ask which known memory arrived before tools.

## Update or remove

For Codex, run `codex plugin marketplace upgrade grasshopper-marketplace`,
then `codex plugin add grasshopper-macos@grasshopper-marketplace` (use your OS
entry). For Cursor, run `agent plugin marketplace update grasshopper-marketplace`,
then update the plugin in Customize. Run the connect skill again with
`--update` so its policy and Cursor CLI MCP path point to the new plugin.
Run `check` and test a fresh session.

Remove the plugin in its harness. Removing a connector does not delete server
memories. Keep the shared config and token file if another agent uses them.

For a manual recovery path, see the [client guide](https://github.com/MylesMCook/grasshopper/blob/main/integrations/plugins/README.md).
