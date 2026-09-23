# Contributing

Read [AGENTS.md](AGENTS.md) and the [shared memory policy](integrations/policy/AGENTS.md) before changing behavior. Grasshopper is a Go-only shared-memory service. The retired Rust code-search implementation is not part of this release.

Run `go test ./...`, `go vet ./...`, and `go test -race ./internal/gomemory ./internal/gomcp ./internal/goclient`. Model-backed recall additionally needs the pinned BGE model and ONNX Runtime paths described in [AGENTS.md](AGENTS.md). Use synthetic databases. Keep scope, revisions, authentication, and recovery assertions intact.

Document actual client/version/OS observations in [acceptance evidence](docs/memory-acceptance.md). A passing unit test does not establish first-turn behavior in a desktop app.
