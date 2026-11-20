Grasshopper 2.14.0

Grasshopper for Pi is now a Pi package.

- Install it with `pi install git:github.com/MylesMCook/grasshopper`. A machine
  that runs only Pi connects with `/grasshopper connect <server link>`; Pi
  downloads the client, checks it against SHA256SUMS, and walks through the
  usual approval.
- Pi's footer shows whether memory loaded. Memory reloads after compaction, and
  `/grasshopper refresh` reloads it any time; `/grasshopper` shows the
  connection check.
- `/remember <text>` saves something on purpose and `/handoff` writes the
  project's handoff.
- The package runs the self-updated client, so it follows the server like
  every other agent. Once it is installed, the client stops writing its own
  Pi extension and removes the old one.
