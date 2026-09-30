# Grasshopper UX audit fixes: site and connector

Audited: `4fea645` (2.7.0 preparation) plus the changes on `codex/ux-audit-site-connector`. Built `web/dist` and served it only at `127.0.0.1:8272`. Playwright CLI 0.1.20 / Chromium on macOS, desktop 1280x900 and phone emulation 375x812, light and dark. The committed memory-view screenshot is a signed-in synthetic view supplied by the report 1 worker; its invented records and device names contain no token. No physical devices or screen readers were used.

Reproduction: `sh web/build.sh`, then `python3 -m http.server 8272 --bind 127.0.0.1 --directory web/dist`. Regression checks: `node --test web/*.test.cjs`, `go test ./cmd/grasshopper ./cmd/grasshopper-go-bundle ./internal/goclient`, and `go vet ./cmd/grasshopper ./cmd/grasshopper-go-bundle ./internal/goclient`. Browser evidence is local under the worktree's ignored `.playwright-cli/` directory. The owned browser session and static server were closed after verification.

## Report 1

### R1-7. Invalid connector address has its own label

- Seen: failing regression checks confirmed invalid URL, nonlocal HTTP, disallowed path, and credentials returned `conflicting_configuration`.
- Impact: agents described an invalid entered link as a saved-configuration conflict.
- Fix: both explicit-address normalization paths return `invalid_address` and a plain `next_step`, retaining the existing validation reason. Existing configuration failures keep their own status.
- Check: `TestConnectInvalidAddressStatus` exercises `connect --json`, checks its status and guidance, and passed after failing before the fix.

### R1-site. Server prerequisite and an actual memory-view preview

- Seen: the home page implied a server; setup placed its hosting instructions last. There was no screenshot.
- Impact: a new owner could install clients before discovering the server prerequisite.
- Fix: home says to start a server first. Setup orders server, connector, then connection. Home shows a responsive synthetic screenshot; setup offers the same preview in a native disclosure. The build copies the image.
- Check: browser assertions verified the first setup heading, successful image loading and no page overflow. The image is 1280x960 and 91 KiB. Its caption explicitly identifies invented memories.

## Report 2: Medium

### R2-7. Setup commands fit phones and match the selected OS

- Seen: source used normal wrapping for long command tokens, lacked copy buttons, and hard-coded macOS plugin names.
- Impact: phone users could lose command text; Windows and Linux users had to rewrite commands.
- Fix: command blocks wrap anywhere. Every block has an accessible Copy button with clipboard support and a select-and-copy fallback. A native OS selector rewrites Codex, Claude Code and Cursor plugin names, plus the archive-route Windows executable spelling.
- Check: all five command blocks had equal scroll and client widths at 375px. Browser selection of Windows updated commands and a real clipboard write reported success. Node tests cover all three OS choices and clipboard denial with selection fallback. No dependency was added.

### R2-8. Ambiguous and malformed server names give actionable errors

- Seen: the exact text `not a url` was already rejected by `new URL` in this build. Bare single-label names, malformed DNS labels, and a bare `?` delimiter were accepted; regression checks failed before the fix.
- Impact: an ambiguous address could navigate to an unintended host.
- Fix: reject ambiguous bare single labels and malformed DNS labels; explain the expected server address. Preserve explicit `https://private-server` links for valid private DNS, complete DNS names, localhost, local IPv4, and IPv6. Continue rejecting credentials, query or fragment delimiters, nonlocal HTTP, and unrelated paths.
- Check: Node tests cover accepted and rejected formats, including uppercase HTTPS, IPv6 and private single-label HTTPS. Browser submission of `not a url` showed the useful address example and stayed on `/view/`.
- Decision: the report's dot-only suggestion would reject legitimate private hostnames supported by the connector. The implementation preserves those when supplied as an explicit HTTPS link.

## Report 2: Low

### R2-9. Public link previews

- Seen: public pages had no Open Graph metadata.
- Impact: shared links offered little context.
- Fix: add title, description, type and canonical page URL to home, setup, view and 404. Home, setup and view also describe and link the synthetic preview image.
- Check: built HTML contains the metadata and the browser read the setup Open Graph title. No external link-preview crawler was tested. Memory-view accessibility changes belong to the other worker's slice.

### R2-11. Agents relay the client's own connection guidance

- Seen: the shared connector skill allowed status paraphrases, and connected/pending guidance was repeated in the client.
- Impact: harnesses could provide inconsistent explanations for the same connection state.
- Fix: the one bundled skill requires agents to quote `next_step` and relay link/device/code separately. Connected and pending messages are shared constants across Connect and Check; successful connection and owner-access guidance use plain wording. Context-specific failure messages continue to describe whether saved access or pending approval was kept.
- Check: connector tests pass; bundle checks verify both Claude and shared Codex/Cursor platform plugins carry the quoting instruction and invalid-address guidance. This verifies the packaged instruction, not fresh live agent compliance.

## Verified OK

- Six Node checks pass; the three Go packages and scoped vet pass; `git diff --check` passes.
- Browser checks cover home/setup at desktop and mobile sizes in light and dark, five unclipped command blocks, OS rewriting, successful clipboard copy, image loading, server-first order, and invalid-address feedback.
- Postel's Law informed retaining explicit private addresses while rejecting ambiguity. Cognitive Load informed a visible server prerequisite and OS commands that need no manual rewriting.
- Public source retains the 2.7.0 native Claude marketplace route and its explicit archive alternative.

## Not tested

Live site deployment, the live Mac memory service, real embedding/search quality, physical phones, screen readers, Windows/Linux browsers, fresh agent interpretation of `next_step`, and external link-preview fetchers. This work does not establish release completion. No shared service, marketplace branch, release, or deployment was changed.
