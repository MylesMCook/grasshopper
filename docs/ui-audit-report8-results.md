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

Implemented U1–U15 and S1–S4 in the existing UI. Both stylesheets use the same
DESIGN color, spacing and type values. The native checkbox stays 20px with a
44px label row; record dividers are removed; filters, results and records share
78ch. Four filters become two columns at tablet widths and one on phones.
Titles open details, review actions share the state region, warnings include
text, and detail metadata uses a definition list. Settings groups Devices,
Export and Sessions without removing the masthead Devices shortcut. Dialog
actions have one primary choice, pending labels survive polling, and revision
navigation hides for a single revision. Setup command strips have keyboard
scrolling and unobscured quiet copy controls; section widths, typography and
dividers follow DESIGN. The existing screenshot is refreshed from synthetic
records; its alt text still describes the visible content.

- Baseline browser reproduced the 134.4×44 checkbox, 960px controls against
  748.8px records, strong record border and 3px heading outline.
- All 107 native Node tests pass (90 memory-view, 17 public-site). New regressions
  were demonstrated failing against the baseline before implementation.
- `go test ./...`, `go vet ./...`, and race tests for memory, MCP and client pass.
  Local results do not claim execution of skipped real-model tests; release CI
  restores pinned model assets.
- Synthetic Chromium via Playwright CLI: memory view at 320, 375, 768, 1024 and
  1280px in both themes; aligned widths, checkbox dimensions, title focus return,
  metadata, one primary action and hidden single-revision navigation pass.
- Keyboard Enter/Space opens titles; Escape returns focus; checkbox Space and
  label activation work. A delayed simulated 503 shows Saving… on the action,
  preserves the draft and permits Keep editing or Discard. Expected network
  errors were confined to that injected failure; no JavaScript exceptions.
- The first memory starts at 651px on a 375×812 viewport. At 320px with text
  enlarged to 200%, the page and detail dialog do not overflow horizontally.
- Copilot identified the long original-author label at enlarged text sizes.
  Shrinkable metadata columns pass 16 combinations across four widths, both
  themes and 100%/200% text, including an actual synthetic owner-edited record.
  Restoring the old max-content track reproduces the overflow.
- Public-site browser assertions pass for 24 page/width/theme combinations, all
  six exact clipboard commands, native horizontal keyboard scrolling, 44px copy
  targets, all three OS choices and feedback reset. System-theme fallback and
  200% text at 320px pass.

Physical devices, real screen readers, other browser engines and fresh native
agent turns were not tested. Release 2.8.2 is deployed after final-head CI/Copilot review and recovery checks.
See the [rollout evidence](memory-acceptance.md#282-report-8-ui-release-and-rollout-september-30).
