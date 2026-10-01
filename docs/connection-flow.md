# Deterministic device connection and recovery

LAB-221. Outcomes approved by the owner on September 29, 2026. One stateless connector,
one authenticated host, existing native approvals and the five memory tools.

## Outcomes

- A connection check reports the host's authenticated credential role and paired
  device ID. A working owner credential is clearly identified as owner access,
  not proof of device registration. Existing memory access is preserved.
- New default Windows connections live under the user's profile in
  `.grasshopper`, outside package-virtualized AppData. Explicit config paths and
  `GRASSHOPPER_CLIENT_CONFIG` remain authoritative. Working legacy connections
  remain readable during upgrade. An explicit connect can reuse a host-verified
  paired credential in the shared location without new pairing; original files
  remain recoverable. Owner/revoked/conflicting credentials are not silently
  copied, replaced or selected as shared device access.
- Pending approval checks read the host once and report pending, ready to finish,
  denied, expired or unavailable accurately. Checks do not create a pairing,
  rewrite credentials or install agent wiring. Successful connect finishes the
  existing approval without another request.
- Host identity mismatch is actionable and does not silently rewrite identity,
  scopes or credentials. Offline recovery keeps current access and pending state.
- Older hosts remain usable; identity is explicitly unverified, rather than
  inventing registration from a successful memory read.

## Acceptance examples

```gherkin
Feature: One dependable device connection

  Scenario: Reuse a registered device across agents
    Given the owner has approved this device's connection
    When an installed agent checks or connects Grasshopper again
    Then it reports the host's registered device ID and server
    And no new approval or credential replacement occurs

  Scenario: Owner access does not masquerade as device registration
    Given an existing agent connection uses the owner's credential
    When the agent checks or connects Grasshopper
    Then it reports owner access and the next action to pair this device
    And it does not report the device as registered
    And current memory access and credentials remain unchanged

  Scenario: Windows agents share one connection
    Given a new default Windows connection is approved
    When packaged Codex and another installed agent use Grasshopper
    Then both resolve the same profile-based connection
    And neither creates an AppData-specific replacement

  Scenario: Reuse an approved legacy Windows connection
    Given a legacy Windows connection is host-verified for this device
    And no conflicting shared connection exists
    When the owner asks the installed agent to connect Grasshopper
    Then the approved device connection is reused in the shared location
    And the legacy files remain recoverable
    And explicit configuration overrides remain authoritative

  Scenario Outline: Report pending approval accurately without side effects
    Given a saved pairing request is <state> on the host
    When the agent checks Grasshopper
    Then it reports <result> and one next action
    And it does not change credentials, configuration or pairing requests
    Examples:
      | state    | result            |
      | pending  | approval pending  |
      | approved | ready to finish   |
      | denied   | approval denied   |
      | expired  | approval expired  |

  Scenario: Preserve access through an outage or identity conflict
    Given a connection or pending approval already exists
    When the host is unavailable or its device ID conflicts with local identity
    Then the agent reports the specific problem and one next action
    And saved credentials and configuration remain unchanged
    And a server switch or replacement still needs the owner's explicit choice

  Scenario: Continue using an older host
    Given the host supports memory access but not connection identity
    When the agent checks Grasshopper
    Then memory access remains usable
    And registration is reported as unverified
```

Native Go service/client tests will enforce these outcomes; no browser test
framework or new dependency is required. Browser pairing controls retain the
already approved matching-code and revocation scenarios.

## Design decision

The recommended path extends the existing host and stateless connector. The
host is the source of truth for credential role, registered device identity and
approval state. The connector holds the address, device credential and temporary
pairing state; it does not infer registration from memory access.

Three approaches were considered:

- Status-only corrections are the smallest patch, but leave Windows agents
  resolving different default connections. They are the first implementation
  step, not the complete outcome.
- Host-reported status plus one stable Windows default addresses both confirmed
  failures with the existing service and client. This is the selected approach.
- A connector repair engine that scans configurations, chooses credentials or
  adds background reconciliation creates more failure paths and surprising
  writes. There is no current need for it.

KISS favors direct calls and the existing architecture. Gall's Law favors
incremental changes to working pairing. Least Astonishment requires truthful
status and explicit credential changes. The Fallacies of Distributed Computing
require bounded remote checks and preservation of local state during outages.

Compatibility is the tradeoff: a small, explicit legacy migration path is
necessary to avoid making working installations reconnect. It must verify
device credentials with the host, preserve originals, honor explicit paths and
stop on conflicts. Rollout uses focused failing-then-passing native tests,
three-OS checks and live client verification. Deployment retains the prior
binaries and configuration; no database migration is expected. Revert the
release on authentication, identity or existing-access regressions.

Remote checks use a five-second HTTP timeout and no automatic retries. Explicit
Connect resumes the same saved pairing request rather than creating duplicates;
the existing request ID provides the idempotency boundary. A status command
returns one non-secret result and next action for observability. Existing host
logs remain the diagnostic path; credentials and private memories are excluded.
Success criteria are one approval for repeated connection attempts, the same
default path across Windows agents, accurate host identity, and zero credential
or configuration writes during checks. No queue or background worker is needed.

## Evidence and boundaries

The current client uses `os.UserConfigDir` and tests connection via MCP context.
Go resolves Windows configuration through AppData and home through USERPROFILE:
[Go implementation](https://go.dev/src/os/file.go). Packaged desktop applications
can redirect AppData writes into per-package LocalCache:
[Microsoft MSIX documentation](https://learn.microsoft.com/en-us/windows/msix/desktop/desktop-to-uwp-behind-the-scenes).
The earlier Windows PC investigation demonstrated separate native packaged and
ordinary connection state; see [verification](verification.md) and LAB-219. Profile
sharing must also be verified on actual native Windows; synthetic paths alone do
not prove packaged-app behavior.

No MCP tool is added. No owner credential is copied into agent configuration.
No registry, network, supervisor, account, or shared-service change occurs during
connection checking. Real-machine results are recorded in [verification](verification.md).
