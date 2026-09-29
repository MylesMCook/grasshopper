# Memory view review and correction

Outcomes approved by the owner on September 29, 2026, from a review of the
owner's whole workflow. Shipped in slices; each row below names its slice.
Backup status is out of scope: the strip shows version, model and counts only.
An owner edit marks the memory confirmed; archive and restore leave
confirmation unchanged. The first-memory exercise is static guidance.

| Slice | Scenarios | State |
| --- | --- | --- |
| 1. Landing scope and list polish | Landing shows everything | Implemented |
| 2. Review and correction | Review queue, correct, conflict, archive and restore, write authorization | Planned |
| 3. Approval, devices and status | Approve a device safely, server status | Planned |
| 4. Startup preview | Preview what an agent receives | Planned |
| 5. Documentation | README and setup trimmed to the user path | Planned |

```gherkin
Feature: Owner reviews and corrects memories in the memory view

  Scenario: Landing shows everything the owner has saved
    Given memories exist in several projects
    When the owner signs in
    Then all memories are listed, each labelled with its project
    And the owner can narrow to one project, device or platform
    And an empty state appears only when nothing is saved anywhere

  Scenario: Review unconfirmed memories
    Given agents have saved unconfirmed memories
    When the owner opens the review queue
    Then unconfirmed memories are listed with their source and date
    And the owner can confirm, correct or archive each one

  Scenario: Correct a memory
    Given the owner opens a memory
    When they edit it and save
    Then a new revision is stored and the earlier one stays viewable
    And the memory shows as confirmed by the owner
    And agents read the corrected text next session

  Scenario: Conflicting edit
    Given an agent corrected the memory after the owner opened it
    When the owner saves their edit
    Then nothing is overwritten
    And the owner sees the newer text and can reconcile

  Scenario: Archive and restore
    When the owner archives a memory
    Then it leaves active lists and agent context
    And it appears under Archived, where it can be restored as a new revision

  Scenario: Writes need a live owner session
    Given an expired session or a request from a foreign origin
    When a change is attempted
    Then it is rejected and nothing changes

  Scenario: Preview what an agent receives
    Given a project, device and platform are selected
    When the owner opens Startup preview
    Then it lists the memories the startup context would include
    And names why others are omitted
    And states it does not prove a session received them

  Scenario: Approve a device safely
    Given an agent requested a connection
    Then the device name and code are prominent and Approve is a primary button
    And the time left is shown
    When the owner opens the approval link while signed out
    Then they can sign in and land back on the same request

  Scenario: Server status
    When the owner is signed in
    Then version, embedding model and record counts are shown
```

## Slice 1 evidence and decisions

- The default view is **All projects**. **Global memories only** and each
  project remain choices. Filters organize the view; they are not access control.
- The list carries a 240-character preview of each memory and flags it with
  `content_truncated`. Full text opens on demand, so a large memory no longer
  pushes others out of the list. Overflow beyond the 32 KiB list budget is still
  disclosed, and each omitted memory is named by title.
- Agent context, `get` and the five MCP tools are unchanged. `Reference` keeps
  its shape; owner-list titles travel in a separate `omitted_titles` field.
- Cards say whether an agent will load a memory. Only confirmed memories and
  handoffs load at startup.
- Tests: `internal/gomcp/visualizer_all_projects_test.go`,
  `visualizer_test.go`, `visualizer/app.test.cjs`.
