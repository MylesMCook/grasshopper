# Go-only transition

Go is the sole implementation for the first shared-memory release. The old Rust CLI and local code-search feature have been retired by user decision. The repository retains a small synthetic SQLite compatibility fixture and historical test notes as migration evidence, not as a second runtime.

The Go server owns scoped SQLite memory, authenticated five-tool MCP, BGE ONNX semantic recall, revision history, backup, and copy-only re-embedding. The Go client is stateless and shared across Codex Desktop, Cursor, and Claude Code. It needs no local database, model, or separate inference service.

Mac, Work HP Windows, and Beelink Ubuntu passed native backend tests, real model recall, copy migration, restart, and Go-only bundle extraction on synthetic data. The Go client passed MCP/context and unavailable-backend checks on all three. Synthetic cross-machine reads and corrections passed, and fresh Mac Codex, Cursor, and Claude Code CLI sessions explained a corrected preference. [Acceptance evidence](memory-acceptance.md) separates these results from deferred app-specific checks. No live deployment, personal profile seeding, or public route has occurred.

[Backend and rollback runbook](shared-memory.md) · [Bundle](go-package.md) · [Client setup](../integrations/README.md)
