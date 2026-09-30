# UX audit follow-up behavior

Owner review requested September 29, 2026. These scenarios are **proposed,
not approved or implemented**. The supplied audit fixes outside these new
behaviors can proceed. Existing review, correction, conflict and archive
outcomes remain in [memory-view-review.md](memory-view-review.md).

```gherkin
Feature: Preserve owner work and make memory views recoverable

  Scenario Outline: Ask before discarding an unsaved correction
    Given the owner has changed a memory's edit form
    When they try to leave the draft by <action>
    Then Keep editing and Discard are offered in the page
    And the draft stays intact until Discard is chosen
    Examples:
      | action                  |
      | Close                   |
      | Escape                  |
      | clicking the backdrop   |
      | Cancel                  |
      | opening another memory  |

  Scenario: Return to a selected memory view
    Given the owner has selected a tab, scope filters and search
    When they reload or use browser Back or Forward
    Then the same view returns from the URL
    And the URL contains no access token

  Scenario: Open a link to a memory
    Given a link identifies a memory
    When the owner opens the link while signed in
    Then that memory opens
    And if sign-in is required it opens after sign-in

  Scenario: Restore an earlier revision safely
    Given an agent has changed a memory
    When the owner restores an earlier revision
    Then a new confirmed revision holds the earlier text and scope
    And prior revisions remain viewable
    And subsequent agent retrieval can read the restored text
    But if the current revision changed nothing is overwritten
    And the owner sees a conflict
```
