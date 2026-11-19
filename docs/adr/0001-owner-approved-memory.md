# Memories load only after the owner keeps them

Most agent memory products extract facts from every conversation on their own. Grasshopper does not: agents save memories explicitly, and a memory loads into later sessions only after the owner keeps it. Coding-agent studies found that automatically captured memory often does worse than no memory, because agents reuse stale or unrelated items without checking them (VibeMemBench, arXiv 2609.23570; MemDev-Bench, 2610.27524; CTIM-Rover, 2505.23422), while verified knowledge helped. Memory is also an injection path (MINJA, 2503.03704), and owner approval keeps an attacker's planted memory out of startup context. Agents save on their own only decisions with their reasons, lessons verified in practice, and handoffs; anything else waits for the owner to ask.

## Considered Options

- Automatic extraction (mem0, Supermemory, Codex memories): more recall, more noise and injection risk.
- Automatic proposals the owner approves: same safety, but a much larger review queue.
