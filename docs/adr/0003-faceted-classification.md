# Memories are classified by facets: area, project and device

Grasshopper is becoming memory for everything its owner does with AI, not only code, so a single global scope would load unrelated memories into every session. Each memory belongs to exactly one area (the part of life it serves, such as work or home) and may also belong to a project and a device. A project names the areas it draws on; a session loads its project's memories, those areas' memories and a small everywhere set, and finds the rest by search. Faceted classification lets one memory be reached from several directions without a deep tree, and the first areas came from sorting the existing memories rather than from a preset list.

## Considered Options

- Projects only: simpler, but a fact shared by several projects must be copied into each.
- Areas only: loses per-repository memory, which most current memories need.

## Consequences

Projects without a Git remote need their own stable identity, so Grasshopper marks such folders with a small identity file. Device stays a facet for facts about one machine; access is not restricted by device.
