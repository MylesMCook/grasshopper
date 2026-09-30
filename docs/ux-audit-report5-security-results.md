# Grasshopper UX audit, report 5: connector credential boundary

Audited: `4fea645` (2.7.0 preparation) with the R5-1 fix on `codex/ux-audit-report5-security`. Codex on macOS. Tests use throwaway configuration directories, invented credentials and loopback `httptest` servers. No live credential, installed connector configuration or service was read or changed.

Reproduction: `go test ./cmd/grasshopper -run 'TestConnect(NeverProbes|SwitchNever|ExplicitOrphan)' -count=1`. Before the fix, four cases recorded one request with an Authorization header at the newly supplied target, zero pairing requests and `authentication_rejected`. No credential values are included in test failures or this report.

## High

### R5-1. An unbound token could be sent to a supplied server

- Seen: `DefaultTokenPath` shared the server's `access-token` filename. Connect interpreted any occupied token output path as a reusable credential and called Identity at the supplied address. A leftover replacement path could cause the same disclosure during a server switch.
- Impact: entering another server link could disclose an owner token or an unrelated saved device credential to that server.
- Fix: the client default is now `device-token` on each OS. Connect reuses credentials only through a saved configuration for the same normalized origin. View/MCP paths, hostname case and default ports normalize for that comparison; scheme and nondefault port remain boundaries. The orphan credential-probing block is removed.
- Fix: an occupied output path gets a new randomly named replacement path using only filesystem metadata. It is never read, overwritten or authenticated. `--token-file` remains an output destination for pairing, rather than authority to authenticate an existing orphan file. Explicit owner bootstrap through Configure/Setup is unchanged.
- Recovery: a pending pairing record explicitly binds its generated secret to one server. If its destination became occupied, Connect preserves that file, updates the pending destination, completes the existing approval and writes the record's generated secret to a fresh file. It does not probe the old file or start another approval. Existing matching-server legacy configurations and verified Windows migration retain their configured credentials; new Windows migration output uses `device-token`.
- Check: security tests cover stray owner/default device files, explicit orphan token paths, switching with a leftover replacement both with and without reconnect, normalized-origin boundaries, and recovery of a pending pairing with an occupied destination. The synthetic target records zero unrelated authenticated requests after the fix. Old config and token bytes remain unchanged during pending switches.

## Accepted scenarios

```gherkin
Scenario: An owner token is beside an unconfigured connector
  Given an owner token exists and no client configuration binds it to a server
  When the owner connects a supplied server link
  Then the connector starts device approval without sending the owner token
  And the owner token remains unchanged

Scenario: A configured device switches servers
  Given this device has saved access to server A
  When the owner explicitly switches to server B
  Then server B receives no existing credential from server A
  And server A's saved files remain unchanged until approval succeeds

Scenario: An occupied pending destination
  Given a pending approval binds a generated device secret to one server
  And its output path is occupied
  When the owner finishes that approval
  Then the connector uses the generated secret at a fresh path
  And the occupied file remains unchanged
```

## Verified OK

- Focused security regressions failed before implementation with an actual authenticated request to the synthetic target, then passed.
- `go test ./cmd/grasshopper ./internal/goclient` passes. This includes same-server repeat connection, legacy migration preservation, pending approval resume and reconnect preservation.
- Scoped vet and race checks pass. The added pending-collision case also passes under race checking.
- The existing test that deleted its configuration and reused an orphan token now requires a new approval and a distinct token path, while preserving the orphan. This intentionally corrects the unsafe compatibility behavior rather than allowing credential discovery to regain a passing test.

## Not tested

Windows/Linux native runtime execution, fresh agent harnesses, real server deployment, live credentials, release archives and published releases. Windows migration assertions were updated for the distinct device-token filename; native Windows CI remains necessary. This local change neither rotates credentials nor revokes any already disclosed token.
