# Portable Go bundle

The optional [live memory view](memory-visualizer.md) is built into the server binary. Enable it with `--visualizer`; it adds no runtime dependency or writable client store.

Task-local archives were built, extracted, checksum-checked, and run on macOS arm64, Work HP Windows x64, and Beelink Ubuntu x64. The Linux pilot used a restored synthetic database and left Beelink's running older service untouched. These are portable package candidates, not signed installers or live deployments.

## Build

Build four Go commands for the target OS: `./cmd/grasshopper` (stateless client), `./cmd/grasshopper-go-server`, `./cmd/grasshopper-go-backup`, and `./cmd/grasshopper-go-migrate`. Build the package tool with `go run ./cmd/grasshopper-go-bundle --help` to see its path flags. The server build uses cgo for ONNX Runtime; the client can be built with `CGO_ENABLED=0`. Installed binaries do not need Go or a compiler on PATH.

Provide the pinned BGE model and tokenizer, matching ONNX Runtime library, runtime license and notices, and all four binaries. The package tool includes the single canonical `integrations/policy/AGENTS.md`, harness configuration examples, setup and visual guides, verification notes, licenses, and `SHA256SUMS`. It refuses to overwrite an existing archive. No database, token, filled-in configuration, or service definition is packaged.

The model and tokenizer must match the SHA-256 digests in `internal/goembed/bge.go`; server startup verifies them. Build separately on each operating system so each archive contains its native ONNX library.

## Versioned client archives

From the repository, run `scripts/build-client-release.sh 2.0.0 /private/output` on macOS or Linux. It builds the stateless client for macOS arm64, Windows x64, and Linux x64, then packages each target's three plugin manifests with the same version and shared `AGENTS.md`. The command refuses to replace an existing archive. Verify each extracted `SHA256SUMS` before installation; the [client guide](../integrations/plugins/README.md) covers a fresh install, update, and removal.

Build the server archive separately on its target OS with the matching ONNX library and pinned model. Client archives do not contain a database, model, token, or server. Rebuild all archives from the final release commit after any code or visual-guide change.

## Try or remove

1. Extract into a new private task-local directory (`unzip` preserves executable bits on macOS and Linux). Check every `SHA256SUMS` entry (`shasum -a 256 -c SHA256SUMS` on macOS, `sha256sum -c SHA256SUMS` on Linux, `Get-FileHash -Algorithm SHA256` on Windows).
2. Keep the token and database **outside** the extracted directory. Start `bin/grasshopper-go-server` with `--db`, `--onnx-library`, `--model`, `--tokenizer`, `--token-file`, and an explicit loopback `--listen`. Add `--create-db` for a new, empty memory database; it never overwrites an existing file. Omit it when opening a converted legacy database. Windows binaries have `.exe`; the ONNX library is `runtime/onnxruntime.dll`. macOS uses `runtime/libonnxruntime.dylib`; Linux uses `runtime/libonnxruntime.so`.
3. Confirm unauthenticated `/healthz` is 401 and authenticated `/healthz` is 200. Use the packaged `bin/grasshopper bridge --config ABSOLUTE_CLIENT_CONFIG` for MCP and `hook` for lifecycle context. Stop the task-local server and remove only that extracted directory to undo the trial.

Use [the recovery runbook](shared-memory.md) before any live cutover. Persistent service installation, private routing, and migration of live data require separate approval and verification.

The fresh database contains no profile. Keep its parent directory private, back it up after accepted writes, and connect clients only through the authenticated `/mcp` endpoint. A loopback trial needs no proxy; remote use needs a private HTTPS route you control.
