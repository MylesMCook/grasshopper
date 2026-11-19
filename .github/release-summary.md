Grasshopper 2.12.1

A clearer memory policy that now reaches every machine.

- Agents save on their own only decisions with their reasons, lessons verified
  in practice, and one handoff at the end of real work. Anything else,
  including preferences, waits until you ask them to remember it.
- Rules for how agents behave belong in AGENTS.md files, not in memory, and
  agents offer to put them there. Documents stay where they live; memory keeps
  a short pointer.
- Handoffs point to the project's task file, branch and commit instead of
  retelling the work.
- Clients now serve the policy built into them, so policy changes arrive with
  the self-update instead of staying at the copy written when a machine was
  first connected.
- New GLOSSARY.md and decision records (docs/adr) describe the memory model
  Grasshopper is moving to.
