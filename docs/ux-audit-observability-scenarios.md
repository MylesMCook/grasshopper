# Structured operational logging acceptance examples

These native-service scenarios implement the owner-supplied R5-2 audit outcome.
Tests are in `internal/gomcp/audit_test.go`; no browser runner or dependency added.

```gherkin
Feature: Diagnose owner and device operations without retaining private data
  Scenario: Follow an owner session and a confirmed memory
    Given structured logging is enabled
    When the owner signs in, confirms a saved memory, and signs out
    Then logs identify session creation and ending
    And the memory action identifies confirm, the memory ID, and the saved revision
    And retries can be correlated using a one-way request ID hash
    And no token, cookie, memory text, title, or raw request ID is logged

  Scenario: Observe device connection outcomes
    Given devices request connection approval
    When the owner approves one request, denies another, and a third expires
    And the owner revokes an approved device
    Then each lifecycle transition is recorded once
    And pairing credentials, codes, hashes, and device names are absent

  Scenario: Collapse repeated authentication failures
    When one peer produces one hundred failed authentications in a minute
    Then one failure entry is recorded
    And the next window's entry reports ninety-nine suppressed failures
    And failure tracking stays bounded as many different peers arrive

  Scenario: Diagnose an unavailable route without recording the error body
    When a route returns a server error
    Then a log identifies its allowlisted route, actual peer IP, and status
    And the raw error message and request query are absent
    And forwarded peer headers are ignored

  Scenario: Keep normal polling quiet
    When owner state polls succeed or a denied pairing request is polled
    Then no per-poll entry is emitted
```
