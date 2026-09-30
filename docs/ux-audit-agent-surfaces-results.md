# Grasshopper UX audit: agent surfaces

Implemented locally against the integrated 2.7.0 audit branch. Synthetic Go
fixtures only; no live agent session or deployed connector was changed.

## R5-7. Scoped lookup guidance

Missing and out-of-scope records return the same `memory_not_found` code and a
hint to check the project, device and platform used for context. Store/archive
failures retain that same boundary. Search describes its scope and 1–100 limit.
The missing-record MCP test failed without the hint before implementation.

## R6-6. Tool-neutral policy

The shipped policy describes task trackers and version control without naming
the owner's tools or hosts. Handoffs should stay under 600 characters with detail
in project files or the task tracker. The product-policy test failed on Linear
and the absent size guidance, then passed after the edit. Repository-only Linear
tracking remains in root AGENTS.md.

## Accepted examples

```gherkin
Scenario: Missing records preserve scope privacy
  Given an agent supplies a scope that cannot read a memory
  When it asks for that memory or a nonexistent ID
  Then both failures explain checking the original context scope
  And neither reveals whether the memory exists elsewhere

Scenario: Installed policy works without the owner's tools
  Given a user installs any Grasshopper connector
  Then its shared policy does not require a personal task tracker or host
  And it gives a concrete size limit for short handoffs
```

## R6-2. Startup budget: selected the policy option; clarified the diagnosis

The audit explicitly offered a numeric short-handoff policy as an alternative
fix. That is implemented above. Retrieval still preserves full record text and
the 3,000-byte Claude budget.

Code and a synthetic regression contradict the proposed displacement mechanism:
confirmed decisions and lessons are selected before handoffs. Five distinct
200-byte confirmed decisions fill this test budget with four records once scope,
provenance and timestamps are included. Adding a 1,700-byte handoff removes none
of those four. The startup preview returns the same IDs and order. Unconfirmed
decisions/lessons remain excluded by the existing accepted startup policy.
Therefore five decisions plus a handoff cannot be promised under this budget;
truncating only the handoff would not make them fit. A richer startup payload is
a separate budget/product change, not hidden in this fix. The real live session
reported in the audit was not independently inspected.
