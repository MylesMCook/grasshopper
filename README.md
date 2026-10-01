# Grasshopper

Memory for your coding agents. Tell one agent something once, and Codex, Cursor
and Claude Code all remember it, on every machine. One private server holds the
memories; you can see, fix or archive anything they keep.

The guided setup is at [usegrasshopper.com/setup](https://usegrasshopper.com/setup/).

<a id="need-a-server"></a>

## 1. Start your server

Already have one? Skip to [Connect an agent](#connect-an-agent).

Download a [server archive](https://github.com/MylesMCook/grasshopper/releases/latest)
(`darwin-arm64`, `windows-amd64` or `linux-amd64`), check it against
`SHA256SUMS`, extract it, and run:

```sh
./bin/grasshopper-server --quickstart
```

On Windows: `.\bin\grasshopper-server.exe --quickstart`

Quickstart creates an empty database and an owner token, and never overwrites
existing state. It prints the memory view address and where the token is. Leave
it running and sign in to the memory view with that token.

To use it from other machines, put it behind a private HTTPS proxy (such as
[Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve) forwarding
to `http://127.0.0.1:8106`, public access off) and restart with:

```sh
./bin/grasshopper-server --quickstart --allowed-proxy-host your-server.tailnet.ts.net:443
```

[Operations](docs/operations.md) covers running it as a service, unsigned
archives, building from source and token recovery.

## Connect an agent

Install a connector on each machine. Every machine uses the same server.

<a id="1-install-a-connector"></a>

### 2. Install a connector

Use `grasshopper-macos`, `grasshopper-windows` or `grasshopper-linux` to match
the machine.

**Codex**

```sh
codex plugin marketplace add MylesMCook/grasshopper --ref marketplace
codex plugin add grasshopper-macos@grasshopper-marketplace
```

**Cursor**

```sh
agent plugin marketplace add https://github.com/MylesMCook/grasshopper.git --git-ref marketplace
```

Then install your entry from the Plugins menu (IDE: **Customize → Plugins**).

**Claude Code**

```sh
claude plugin marketplace add MylesMCook/grasshopper#marketplace
claude plugin install grasshopper-macos@grasshopper-marketplace
```

Approve the three read tools the first time Claude Code asks. To pre-approve
them instead, use the [client archive](https://github.com/MylesMCook/grasshopper/releases/latest)
in place of the two commands above (one route or the other, not both):

```sh
./bin/grasshopper connect --agents claude --url https://your-server.tailnet.ts.net
```

<a id="2-connect-once"></a>

### 3. Connect once

In a new agent session, say **Connect Grasshopper**. Give it your server address
if asked, then open the approval link in your memory view, check the device and
code match, and approve. No token passes through the agent.

Codex may ask you to trust its hook and allow network access; Cursor may ask you
to turn on Grasshopper MCP. Start a new session afterwards. To check later, ask
the agent to check Grasshopper.

<a id="3-try-it"></a>

### 4. Try it

Ask the agent to remember a preference, start a new session, and ask about it.
Then find it in your memory view.

## Use the memory view

Sign in on your server as the owner. The sidebar has **New** (what agents saved
that you have not kept yet; agents load it at startup only after you keep it.
Handoffs are the exception: the latest one loads without being kept),
**All projects**, your projects, **Agents**, **Archived** and **Settings**.
Open a memory to edit it, move it between a project and All projects, or
archive it; earlier versions stay in its history. Each project keeps only its
latest handoff, and older ones move to Archived. Projects change what you see,
not who can access it.

## Update or remove a connector

Connector changes never delete memories on the server.

To see which connectors need an update, run `./bin/grasshopper check` from the
client archive. It lists each agent's connector version, whether it matches the
server, and the exact update step for any that are behind. It fails only when an
agent points at a Grasshopper program that no longer exists.

| Agent | Update | Remove |
| --- | --- | --- |
| Codex | `codex plugin marketplace upgrade grasshopper-marketplace`, then `codex plugin add` your entry again | Remove the entry in the Codex plugin manager |
| Cursor | `agent plugin marketplace remove grasshopper-marketplace`, add it again, reinstall your entry | Run `./bin/grasshopper cursor remove` from the client archive, then uninstall the plugin |
| Claude Code | `claude plugin marketplace update grasshopper-marketplace`, then `claude plugin update` your entry | `claude plugin uninstall` your entry, then `claude plugin marketplace remove grasshopper-marketplace` |
| Claude Code (client archive) | `./bin/grasshopper setup --agents claude --update` | `./bin/grasshopper claude remove`, then remove the plugin |

You can disconnect one device in the memory view without affecting others. A
device token can read the whole store; project scope is not access control.

## Back up and update the server

Before replacing a server, run the backup tool from the **new** server archive
and keep an encrypted copy off the machine:

```sh
./bin/grasshopper-backup --quickstart
./bin/grasshopper-backup --verify /absolute/path/to/backup.db
```

Stop the old server, keep its archive and database, and never run two servers
on one database. [Operations](docs/operations.md#update-and-recover) covers
restoring a backup, rolling back and the one-time re-embedding when upgrading
from 2.3.x or earlier.

## Develop

```sh
go test ./...
go vet ./...
node --test internal/service/visualizer/app.test.cjs web/*.test.cjs
```

[AGENTS.md](AGENTS.md) has the layout and project rules, and
[DESIGN.md](DESIGN.md) the visual system. [Verification](docs/verification.md)
lists what has been checked on real machines. The
[shared memory policy](integrations/policy/AGENTS.md) governs all three agents.

[Report security concerns privately](https://github.com/MylesMCook/grasshopper/security/advisories/new),
not in a public issue.
