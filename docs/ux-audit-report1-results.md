# Grasshopper UX audit, report 1 fix results

Audited: local changes based on `4fea645` (2.7.0 prep), in the isolated
`codex/ux-audit-report1` worktree. Embedded memory view served against a synthetic
SQLite store with 32 invented records, a test-only owner token, wording search,
and a 32,768-byte memory. Browser: Playwright CLI's Chromium on macOS,
375×812 mobile emulation and 1280×960 desktop, light and dark. No real screen
reader or physical device was used. These checks do not establish deployed
behavior or release completion.

Reproduction: task-local harness at
`~/Documents/Codex/2026-09-29-ux-audit-report1/harness/main.go`; its `go.mod`
points to this worktree. Build with `GOWORK=off go build -o synthetic-view .`,
then run `./synthetic-view` to serve `127.0.0.1:8198`. It uses invented
`id:workshop`, `id:orchard`, `test-desktop`, and `studio-laptop` identities.
The test token is the report's synthetic token; no production credential was
used. Each harness run creates fresh disposable databases.

The saved browser assertion script is
`output/playwright/report1-checks.cjs` in the local worktree. Run it in an
already signed-in isolated Playwright CLI session with
`playwright-cli -s=gh-report1 run-code --filename=output/playwright/report1-checks.cjs`.
Browser screenshots and scripts are local evidence, not committed test fixtures.

## Memory view

### R1-1. Phone first screen contains memories

- Seen: the report placed tabs at 795px and the first memory at 1,054px.
- Impact: the main task was hidden behind setup controls.
- Fix: tabs precede search and filters. On screens at most 700px wide, a
  **Filters** button expands the project, device, platform, and explanatory
  note. Desktop filters remain visible. Search stays available in Saved.
- Check: Chromium at 375×812 placed tabs at 229px, the first memory at 615px,
  and its text preview at 704px. Filters started closed and expanded with
  `aria-expanded=true`. The saved browser script asserts these outcomes.

### R1-2. Bound the initial list

- Seen: 32 memories made the report's phone page 11,966px tall.
- Impact: browsing and reaching later controls required excessive scrolling.
- Fix: initially display eight records, reveal eight more with **Show more
  memories**, and keep that count across live redraws. Mobile card text is
  limited visually to three preview lines; Open memory retains the full text.
  Search and scope changes start again at eight. The total summary still names
  all results; the button names the remaining count.
- Check: the 32-record harness initially displayed eight cards on a 3,014px
  page. Node and browser checks verify eight then sixteen without a new
  network request for Show more, and Node verifies a poll keeps sixteen.
  The server's existing list-size omission disclosure remains unchanged.

### R1-3. Find devices from the masthead

- Seen: Devices & connections was below the entire memory list.
- Impact: connecting a new device was difficult once the store had data.
- Fix: a signed-in **Devices** masthead link opens the panel, scrolls to it,
  and focuses its summary. Approval links retain their existing prominent
  panel placement. Signed-out users do not see an unusable Devices link.
- Check: Node and Chromium verify the panel opens from the masthead.

### R1-4. Keep dialog controls reachable

- Seen: Close scrolled away and write actions could fall beyond the phone.
- Impact: long memory reads and corrections hid their next action.
- Fix: only the dialog body scrolls. The header holds Close; the footer holds
  memory actions or Save and confirm/Cancel while editing. Full text,
  provenance, and revision navigation remain in the scrolling body.
- Check: a 32,768-byte memory at 375×812 keeps Close at 25–69px and memory
  actions at 743–787px before and after scrolling 29,321px of dialog content.
  Save and confirm/Cancel also remain at 743–787px. Browser assertions verify
  controls stay within the viewport. Existing full-text/history checks pass.

### R1-5. Review directly from the list

- Seen: each review needed Open memory followed by Confirm.
- Impact: a queue required repeated navigation for routine confirmation.
- Fix: Needs review cards have **Confirm** and **Archive**. Confirmation
  fetches full saved text, checks the displayed revision is still current,
  and posts through the existing owner update API. Archive uses the existing
  archive API. Both retain expected revisions and request IDs; an uncertain
  retry reuses the identical payload. A changed record is refreshed for
  review instead of overwritten. Reads from a former sign-in cannot write
  into a new session. Poll redraws do not leave replacement buttons disabled.
- Check: Chromium confirmation removed the invented observation from Needs
  review and changed its count from one to zero without opening a dialog.
  Node covers full text despite a shortened preview, both action payloads,
  concurrent revisions, write conflicts, uncertain retries, poll redraws,
  and sign-out/sign-in during a read. Existing server write authorization,
  conflict, and archive/restore checks pass in `go test ./internal/gomcp`.

### R1-6. Explain sign-in failures where they can be corrected

- Seen: token rejection appeared below the form beside a contradictory prompt.
- Impact: neither the rejected field nor a practical recovery step was clear.
- Fix: a live alert next to the token field explains rejection and returns
  focus to that field with `aria-invalid=true`. The summary says sign-in
  failed. Help names the server operator as the person to ask privately,
  distinguishes owner and device tokens, and warns against sending a token
  to an agent chat. A server operator can still use the owner token file
  named at startup. No new token retrieval facility is implied.
- Check: Node and Chromium verify wrong-token field focus, invalid state,
  adjacent error text, and the corrected summary. A subsequent valid
  synthetic sign-in clears the error and loads memories.

## Verified OK

- `node --test internal/gomcp/visualizer/app.test.cjs`: 44 tests, 44 pass,
  none skipped. Three new regressions were first demonstrated failing against
  the original implementation, then passed after the changes.
- `go test ./internal/gomcp`: pass, including owner authorization, conflict,
  list, session, and archive/restore server checks.
- `node --check internal/gomcp/visualizer/app.js` and `git diff --check`: pass.
- Browser assertions: mobile first-screen content, filters, Show more,
  masthead Devices, full long-memory reading, pinned read/edit actions, and
  desktop filter visibility. Captured and inspected mobile light and desktop
  light screenshots; captured dark equivalents. No horizontal overflow at
  375px or 1280px in the observed states.

## Not tested and remaining scope

- Real screen readers, physical phones, Windows/Linux browsers, real-model
  search quality, and the live service were not tested.
- Report 1 item 7 (connector status label) and public-site ordering/screenshot
  belong to the separate connector/site worker. Report 2 fixes are separate.
- No release version, live service, marketplace, public deployment, PR,
  merge, or push was changed by this worker.
