# Rules live in instruction files, not in memory

Instructions for how agents must behave belong in the AGENTS.md files that every harness loads on its own, never in Grasshopper. Native instruction files load completely every session, while memory arrives through a budgeted startup hook that can cut records and cannot always report what it dropped (a short Claude Code startup budget already cut most kept preferences). Two copies of the same rule also drift apart. Grasshopper keeps what agents learn: facts, preferences, decisions with reasons, lessons, pointers and handoffs. A memory that turns out to be a rule moves into the instruction files and is archived here.

## Considered Options

- Edit rules in Grasshopper and write them out to instruction files: one editing place, but a second sync system to build and trust.
- Rules only in memory: simplest model, but rules can be cut or fail to load.
