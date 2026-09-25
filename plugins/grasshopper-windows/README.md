# Grasshopper for windows

This connector uses your private server. It has no memory database or token.

Ask your agent: **Use the connect-grasshopper skill.** Give it the server's HTTPS `/mcp` address and the path to a token file on this machine. Keep the token out of chat.

You can also run `bin/grasshopper.exe setup --agents none --url ADDRESS --token-file PATH`. For Cursor Agent CLI, add `--cursor-cli`. The command checks authentication before saving config. Then approve the native hook or MCP prompt and test a fresh session.
