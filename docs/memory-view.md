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
  the five most recent projects plus the open one, and "N more" listing the
  rest by name, then Agents with status dots, Archived, Settings. The order is
  fixed when the view loads, so the owner's own changes never reshuffle it. On
  phones it becomes a menu at the top.
- The main column: the page title, search, and sections labelled in a left
  margin by where they apply, or by date for the latest handoff. Each memory is
  one line with "source · date" on the right. Large lists group by This week,
  the month, then Earlier, with "Show N more"; each group's label keeps the
  place ("grasshopper · This week"). New lists projects by name, so deciding
  one never moves the rest.
- A project page shows what its agents start with: the latest handoff first,
  then its own memories and the memories for every project (folded only past 8,
  and never when the project has none of its own). An empty project section
  shows only when nothing applies. The subtitle counts them, followed by "Add
  memory" and "Preview startup", which opens the exact startup context.
- Archived lists what the owner archived. Handoffs replaced by newer ones fold
  into one "Show N replaced by newer ones" line at the end.
- Opening a memory adds a right-hand panel on wide screens, replaces the list
  on laptops, and becomes a page on phones. It shows the full text, Edit,
  Move to…, Archive, then Applies to, Startup (Loads, Not until you keep it,
  Not loaded), Saved by and History. Each row's detail is "Codex · Oct 6";
  handoffs start with "handoff". A device restriction hides the platform it
  implies. History shows each version's text, and its title where that differs,
  and appears only when an earlier version differs. Escape closes the panel and
  focus returns to the row that opened it. Archive and restore keep the
  memory's author; the audit log records the owner's action.
  The owner's own saves show "You" and the date, without internal provenance.
- Move to is a dropdown of All projects and durable projects by name, starting
  at where the memory applies now; Move does nothing until another place is
  picked.
- Add memory and Edit share one form. The text box grows with the text;
  Cmd/Ctrl+Enter saves and Escape cancels. A memory the owner adds is kept at
  once and saved as a preference for the chosen project or All projects.

- The owner can name a device in its panel. The name shows wherever the
  device appears; its ID, used in tokens, scopes and provenance, never changes
  and stays visible in the panel. An empty name shows the ID again.

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

  Scenario: A memory moves to another project or every project
    Given a kept memory in project "grasshopper"
    When the owner moves it to All projects or to project "tempus"
    Then a copy with the same text, purpose and source applies there
    And the original is archived with a note naming the copy
    And retrying the same move does not create a second copy
    And a device or platform restriction stays as it was

  Scenario: The owner adds a memory
    Given the owner is on project "grasshopper"
    When the owner adds "Prefer short answers."
    Then it applies to "grasshopper" unless the owner picks another place
    And it is kept, so agents load it at startup
    And retrying the same save does not create a second memory

  Scenario: A project page shows what agents start with
    Given 9 memories apply to every project and none only to "grasshopper"
    When the owner opens "grasshopper"
    Then all nine are listed without unfolding or an empty "grasshopper" section
    And the subtitle says "9 memories apply here"
    And "Preview startup" opens the startup context for "grasshopper"

  Scenario: Replaced handoffs do not bury archived memories
    Given the owner archived one memory and 12 handoffs were replaced
    When the owner opens Archived
    Then the archived memory is listed with Restore
    And the replaced handoffs fold into "Show 12 replaced by newer ones"

  Scenario: The owner names a device without changing its ID
    Given a paired machine with the ID "HPLT-1234"
    When the owner names it "Work laptop"
    Then Agents and memories scoped to it show "Work laptop"
    And its panel still shows "ID: HPLT-1234"
    And a replacement credential for that machine keeps the name

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
`internal/service/visualizer_move_test.go` and
`internal/service/visualizer_create_test.go`. Browser behavior is covered by
`internal/service/visualizer/app.test.cjs`.
