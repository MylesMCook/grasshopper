# Portable Grasshopper bundle

The public [Grasshopper site](https://usegrasshopper.com/) explains how it works and how to set it up. The private server serves only the optional [live memory view](memory-visualizer.md) at `/visualizer/` when enabled. It adds no writable client store.

The [latest published release](https://github.com/MylesMCook/grasshopper/releases/latest) has checksum-checked, unsigned portable archives for macOS, Windows, and Linux. See [verification](memory-acceptance.md) for which binaries actually ran on each operating system. The Mac mini private service is a separate installation from the release archive. Check the published release version before using the `--quickstart` command below; it starts with 2.1.0.

## Build

Build four commands for the target OS: `./cmd/grasshopper` (stateless client), `./cmd/grasshopper-go-server`, `./cmd/grasshopper-go-backup`, and `./cmd/grasshopper-go-migrate`. Build the package tool with `go run ./cmd/grasshopper-go-bundle --help` to see its path flags. The server build uses cgo for ONNX Runtime; the client can be built with `CGO_ENABLED=0`. Installed binaries do not need a compiler or language toolchain on PATH.

Provide the pinned BGE model and tokenizer, matching ONNX Runtime library, runtime license and notices, and all four binaries. The package tool includes the single canonical `integrations/policy/AGENTS.md`, harness configuration examples, setup and visual guides, verification notes, licenses, and `SHA256SUMS`. It refuses to overwrite an existing archive. No database, token, filled-in configuration, or service definition is packaged.

The model and tokenizer must match the SHA-256 digests in `internal/goembed/bge.go`; server startup verifies them. Build separately on each operating system so each archive contains its native ONNX library.

## Versioned client archives

From the repository, run `scripts/build-client-release.sh VERSION /private/output` on macOS or Linux. It builds the stateless client for macOS arm64, Windows x64, and Linux x64, then packages each target's three plugin manifests with the same version and shared `AGENTS.md`. The command refuses to replace an existing archive. Verify each extracted `SHA256SUMS` before installation; the [client guide](../integrations/plugins/README.md) covers a fresh install, update, and removal.

Build the server archive separately on its target OS with the matching ONNX library and pinned model. Client archives do not contain a database, model, token, or server. Rebuild all archives from the final release commit after any code or visual-guide change.

## Try or remove

1. Extract into a new private task-local directory (`unzip` preserves executable bits on macOS and Linux). Check every `SHA256SUMS` entry (`shasum -a 256 -c SHA256SUMS` on macOS, `sha256sum -c SHA256SUMS` on Linux, `Get-FileHash -Algorithm SHA256` on Windows).
2. From the extracted directory, run `./bin/grasshopper-go-server --quickstart` on macOS or Linux, or `.\bin\grasshopper-go-server.exe --quickstart` in Windows PowerShell. It creates an empty database and private token in your user Grasshopper data directory, then prints the local memory-view address and token-file path. Open the token file locally to connect the view; do not put its contents in chat, Git, or a command line. A second run reuses complete state and never resets it. Use `--data-dir ABSOLUTE_PATH` for an isolated trial. Stop with Ctrl-C.
3. Confirm unauthenticated `/healthz` is 401 and authenticated `/healthz` is 200. Use the packaged `bin/grasshopper bridge --config ABSOLUTE_CLIENT_CONFIG` for MCP and `hook` for lifecycle context. Removing the extracted directory removes only the program; remove a task-local `--data-dir` separately if you want to discard its synthetic trial data.

Existing or converted databases still use manual flags: `--db`, `--onnx-library`, `--model`, `--tokenizer`, `--token-file`, and explicit loopback `--listen`. `--create-db` initializes a missing database and never overwrites an existing file. A private HTTPS proxy that preserves its Host header also needs `--allowed-proxy-host` set to that exact hostname and port. Quickstart does not add remote access, automatic startup, or backups.

Use [the recovery runbook](shared-memory.md) before any live cutover. Persistent service installation, private routing, and migration of live data require separate approval and verification.

The fresh database contains no profile. Keep its parent directory private, back it up after accepted writes, and connect clients only through the authenticated `/mcp` endpoint. A loopback trial needs no proxy; remote use needs a private HTTPS route you control.
