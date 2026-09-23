# Portable Go bundle

A task-local archive was built, extracted, checksum-checked, run, and removed on macOS arm64 and Work HP Windows x64. This is a portable package candidate, not a signed installer or a live deployment.

## Build

Build four Go commands for the target OS: `./cmd/grasshopper` (stateless client), `./cmd/grasshopper-go-server`, `./cmd/grasshopper-go-backup`, and `./cmd/grasshopper-go-migrate`. Build the package tool with `go run ./cmd/grasshopper-go-bundle --help` to see its path flags. The server build uses cgo for ONNX Runtime; the client can be built with `CGO_ENABLED=0`. Installed binaries do not need Go or a compiler on PATH.

Provide the pinned BGE model and tokenizer, matching ONNX Runtime library, runtime license and notices, and all four binaries. The package tool includes the single canonical `integrations/policy/AGENTS.md`, harness configuration examples, setup guide, first-party/model/runtime/Go dependency notices, and `SHA256SUMS`. It refuses to overwrite an existing archive. No database, token, filled-in configuration, or service definition is packaged.

The model and tokenizer must match the SHA-256 digests in `internal/goembed/bge.go`; server startup verifies them. Build separately on each operating system so each archive contains its native ONNX library.

## Try or remove

1. Extract into a new private task-local directory (`unzip` on macOS preserves executable bits). Check every `SHA256SUMS` entry (`shasum -a 256 -c SHA256SUMS` on macOS, `Get-FileHash -Algorithm SHA256` on Windows).
2. Keep the token and database **outside** the extracted directory. Start `bin/grasshopper-go-server` with `--db`, `--onnx-library`, `--model`, `--tokenizer`, `--token-file`, and an explicit loopback `--listen`. Windows binaries have `.exe`; the ONNX library is `runtime/onnxruntime.dll`. macOS uses `runtime/libonnxruntime.dylib`.
3. Confirm unauthenticated `/healthz` is 401 and authenticated `/healthz` is 200. Use the packaged `bin/grasshopper bridge --config ABSOLUTE_CLIENT_CONFIG` for MCP and `hook` for lifecycle context. Stop the task-local server and remove only that extracted directory to undo the trial.

Use [the recovery runbook](shared-memory.md) before any live cutover. Persistent service installation, private routing, and migration of live data require separate approval and verification.
