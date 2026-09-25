# Grasshopper for Codex

This is the Git-backed Codex marketplace plugin. It adds Grasshopper's MCP tools and startup hooks to Codex. For the tested local package route, use the [client guide](https://github.com/MylesMCook/grasshopper/blob/main/integrations/plugins/README.md) instead. Install **one** Codex route, not both.

## Install

1. [Run your private server](https://usegrasshopper.com/setup/) and [configure a client](https://github.com/MylesMCook/grasshopper/blob/main/integrations/plugins/README.md) on this machine.
2. Put that client's `grasshopper` binary on the PATH seen by Codex CLI **and** Codex Desktop. On Windows, use `grasshopper.exe`.
3. Add this repository as Grasshopper's marketplace and install the plugin:

   ```sh
   codex plugin marketplace add MylesMCook/grasshopper --ref main
   codex plugin add grasshopper@grasshopper-marketplace
   ```

4. In a fresh Codex task, review and trust the hooks in `/hooks`. Check that a known record arrived before tool use. If startup context is absent, the shared policy calls for one MCP `context` attempt.

This route does not start the server, copy a token, or hold memories. Codex Desktop may use a different PATH from your terminal, so test it separately. The archive-local plugin passed fresh CLI sessions; a fresh Git-backed marketplace install has not yet been verified.

## Remove

```sh
codex plugin remove grasshopper@grasshopper-marketplace
codex plugin marketplace remove grasshopper-marketplace
```

Keep the client, config, token file, and shared policy if Cursor or Claude Code still uses them. Removing the plugin does not delete server memories.
