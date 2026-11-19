Grasshopper 2.12.0

Pi joins Codex, Cursor and Claude Code.

- Grasshopper connects Pi with one extension in Pi's user folder. Pi loads
  memory at startup and gets the five memory tools, run by the same
  self-updated client as every other agent on the machine.
- No plugin step: once any agent on a machine runs this client, it adds Pi's
  extension at its next session when Pi is installed, and rewrites it when a
  later client changes it.
- `grasshopper pi install` connects Pi right away, including on a machine that
  runs only Pi. `grasshopper pi remove` disconnects it, and agents leave Pi
  alone until it is installed again.
- `grasshopper check` lists Pi with the client it runs.
