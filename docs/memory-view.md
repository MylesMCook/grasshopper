# Memory view (2.9.0)

The owner-approved design from the 2.9.0 prototypes. The stored data and the
five agent tools do not change; this page decides what the owner sees.

## Model

- **Kept**: a confirmed memory. Agents load it at startup.
- **New**: an unconfirmed memory other than a handoff. Agents do not load it at
  startup until the owner keeps it. Keep confirms it; Discard archives it.
- **Handoff**: one per exact scope. Saving a new handoff archives the previous
  one with a note naming its replacement.
- **Where it applies**: a project, or All projects (global). Device and
  platform scopes still exist and show only when a memory has them.
- **Who saved it**: "you" for the owner's edits and kept memories the owner
  wrote; otherwise the agent from its provenance.

## Layout

- A permanent sidebar: New (only when there is something new), All projects,
  the five most recent projects and "N more", Agents with status dots,
  Archived, Settings. On phones it becomes a menu at the top.
- The main column: the page title, search, and sections labelled in a left
  margin by where they apply, or by date for the latest handoff. Each memory is
  one line with "source · date" on the right. Large lists group by This week,
  the month, then Earlier, with "Show N more".
- Opening a memory adds a right-hand panel on wide screens, replaces the list
  on laptops, and becomes a page on phones. It shows the full text, Edit,
  Move to…, Archive, then Applies to, Saved by and History.

## Scenarios

```gherkin
Feature: Owner reviews and manages memories

  Scenario: New suggestions are reviewed in place
    Given an agent saved an unconfirmed decision in project "grasshopper"
    When the owner opens the project
    Then a "New" section lists it with Keep and Discard
    And Keep confirms it, so agents load it at startup
    And Discard archives it, so it appears under Archived as discarded

  Scenario: A suggestion that repeats a kept memory is flagged
    Given a kept memory shares most of its words with a new suggestion
    When the owner opens the suggestion
    Then the kept memory is shown as similar
    And "Keep and replace it" keeps the suggestion and archives the old one

  Scenario: Each scope keeps one handoff
    Given project "grasshopper" has a handoff
    When an agent saves a newer handoff for "grasshopper"
    Then the older handoff is archived with a note naming the newer one
    And handoffs in other scopes are unchanged

  Scenario: A memory moves between a project and every project
    Given a kept memory in project "grasshopper"
    When the owner moves it to All projects
    Then a copy with the same text, purpose and source applies everywhere
    And the original is archived with a note naming the copy
    And retrying the same move does not create a second copy

  Scenario: Agents show when each machine last connected
    Given a paired machine authenticated two minutes ago
    When the owner opens Agents
    Then the machine shows as active with its last-seen time
    And a revoked machine shows no activity

  Scenario: Edits never overwrite newer changes
    Given an agent changed a memory while the owner was editing it
    When the owner saves
    Then nothing is overwritten
    And the owner sees both versions and keeps one
```

Server behavior is covered by `internal/memory/handoff_retention_test.go`,
`internal/memory/client_tokens_test.go` and
`internal/service/visualizer_move_test.go`. Browser behavior is covered by
`internal/service/visualizer/app.test.cjs`.
