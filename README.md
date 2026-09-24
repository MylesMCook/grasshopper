# Grasshopper

Grasshopper is a self-hosted memory service for Codex, Cursor, and Claude Code. One authenticated SQLite backend holds explicit preferences, accepted decisions, verified lessons, and handoffs. Each harness connects through the same stateless client. [Download 2.0.1](https://github.com/MylesMCook/grasshopper/releases/tag/v2.0.1) · [See how it works](docs/how-it-works.html).

**Status:** Native builds and synthetic CLI checks passed on Mac, Work HP Windows, and Beelink Ubuntu. The Mac mini now hosts a private service with encrypted offsite backups; no old database was migrated. Windows Codex uses a first-prompt hook, and Cursor CLI needs project MCP wiring. Desktop checks remain user-led. [See exact evidence](docs/memory-acceptance.md).

A new host can start an empty memory-only SQLite database with `grasshopper-go-server --create-db --db /private/brain.db` plus the model, runtime, token-file, and loopback flags described in the [portable bundle guide](docs/go-package.md). The flag never replaces an existing database. Remote access still needs an approved private HTTPS route.

## What it remembers

- Global confirmed preferences that follow the user between projects.
- Project decisions keyed by normalized Git remote or an explicit durable ID.
- Device and OS facts that apply only where they belong.
- Short handoffs tied to verified work.

`context` loads applicable whole records directly. `store` requires an explicit scope and source; a correction uses a stable ID/key plus expected revision. `search` uses scoped keyword and local ONNX vector recall. `get` returns the full record or a historical revision. `archive` reversibly removes an active record. Similarity never silently replaces content. Unknown-scope legacy records stay out of normal recall.

## Build and test

Use Go 1.27.1 or the version in `go.mod`. The server needs the pinned BGE ONNX model, tokenizer, and matching ONNX Runtime library. Clients need neither the model nor a local database.

```sh
go test ./...
go vet ./...
go test -race ./internal/gomemory ./internal/gomcp ./internal/goclient
go build -o /your/build/grasshopper ./cmd/grasshopper
go build -o /your/build/grasshopper-go-server ./cmd/grasshopper-go-server
go build -o /your/build/grasshopper-go-backup ./cmd/grasshopper-go-backup
go build -o /your/build/grasshopper-go-migrate ./cmd/grasshopper-go-migrate
```

[Portable server bundle](docs/go-package.md) explains how to assemble and verify a server archive. [Client plugin packages](integrations/plugins/README.md) cover installation, updates, and removal; [manual wiring](integrations/README.md) remains available. [Live memory view](docs/memory-visualizer.md) shows current context without a second store. [Mac mini deployment and recovery](docs/mac-mini-deployment.md) records the private host setup.

The retired Rust code-search CLI is intentionally absent. Use the repository's normal code tools for code search. Grasshopper's remote surface remains memory-only.

## Security and operation

The server binds to an explicit loopback address and requires a bearer token, including for health checks. Put credentials in approved secret management, not Git, CLI arguments, or logs. Remote access needs a separately approved private HTTPS route. Project scope prevents accidental mixing; it does not authorize access to data. Restricted employer data needs a separate access boundary or deployment.

[AGENTS.md](AGENTS.md) governs repository work; [the one shared memory policy](integrations/policy/AGENTS.md) governs memory use in all three harnesses. No alternate standing-instruction format is generated.
