# UI audit report 8

Owner report accepted against 2.8.1 (`bf37573`). Codex coordinates isolated
Sol workers; no new dependencies or backend contracts. Changes remain in the
existing stylesheets and markup, with narrow rendering logic for controls.

## Accepted outcomes

```gherkin
Scenario: Read and review memories on a phone or desktop
  Given a signed-in owner has saved and unconfirmed memories
  When they browse in either theme
  Then filters, results and records share one reading width
  And titles open details with visible keyboard focus on controls
  And review actions and unconfirmed states are clear without color alone

Scenario: Edit without losing feedback or recovery
  Given the owner opens a memory
  Then metadata has labels and one action has primary emphasis
  When the owner saves a change
  Then the action shows progress until the request finishes
  And failures preserve the draft and allow retry

Scenario: Export and manage browser sessions
  Given the owner opens settings
  Then devices, export and sessions have distinct headings
  And Include archived is a native checkbox with an aligned label

Scenario: Copy a setup command at a narrow width
  Given the owner chooses an operating system
  Then long commands remain intact in a keyboard-scrollable block
  When they use its copy control
  Then only the complete command is copied
  And feedback resets after copying or changing operating system
```

## Verification

Pending implementation, native regressions and synthetic browser checks at
mobile, tablet and desktop widths in light and dark themes. Physical devices,
real screen readers and other browser engines are outside this pass.
