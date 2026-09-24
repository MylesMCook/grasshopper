# Research check: changing memories

**Question:** When a preference changes, can Grasshopper show what is true now without losing what used to be true?

## Papers worth using

Found through alphaXiv, then checked against the papers themselves.

| Paper | Useful idea | Fit for Grasshopper |
|---|---|---|
| [A-TMA (2026)](https://arxiv.org/abs/2607.01935) | Test the memory bank, retrieval, and final answer separately. Old but relevant facts can mislead an agent if their state is unclear. | Directly relevant to corrected preferences and project decisions. Its graph and controller are unnecessary for this small store. |
| [MemTxn (2026)](https://arxiv.org/abs/2607.27834) | Check source support, conflicts, and recovery before treating a write as durable. | Reinforces provenance, expected revisions, and backup tests. Its learned admission layer would add a model dependency we do not want. |
| [Agent Memory systems study (2026)](https://arxiv.org/abs/2606.06448) | Measure read and write cost separately instead of judging only answer accuracy. | Useful for a later latency and context-size check on the real client path. |

These are research leads, not proof that a method will work in our three harnesses. None justifies a new graph, vector database, or memory-writing agent here.

## Synthetic baseline

`go test -count=1 -run TestStateEvolutionCurrentVsHistory -v ./internal/gomemory` passed on macOS arm64 with the copied SQLite fixture. The test corrected a confirmed global preference from revision 2 to 3, then checked:

- `context` returned revision 3 in both the original and another project; active lexical `search` returned revision 3.
- Wording found only in revision 1 did not enter active lexical search.
- Scoped `get` returned revision 1 with its provenance.
- Full-record `get` required the record's exact global scope, including for its prior revision.

The result supports the **bank and lexical retrieval** parts of this question. Historical `get` requires a record ID and revision; active search intentionally does not index old wording. Separate real-model tests cover semantic recall of current records.

**Three real CLI passes on macOS arm64:** Codex CLI 0.155.1, Cursor Agent CLI 2026.09.23-86fc751, and Claude Code 2.1.280 used the native Go bridge and a disposable, authenticated Go server with real BGE. Each fresh, read-only session fetched record 1 revisions 1 and 2, correctly reported “concise” → “direct,” retained the source-reference requirement, and cited provenance. Codex called `context` then `get`; Cursor and Claude called `get` for both revisions without a `context` call in their tool traces. No memory write occurred. Event traces are in the task-local `2026-09-23-grasshopper-temporal-experiment/run/` folder.

Cursor's first headless run rejected `get` as unapproved and looped. A second run passed after a **project-local, read-only MCP allowlist** was added to `.cursor/cli.json`; no global permission setting changed. This is evidence for the CLI configuration, not for the Cursor desktop app.

## Next experiment

Repeat the state-change probe on Work HP through the Cursor and Claude Code CLIs, then test a longer correction chain and an ambiguous historical question. If an agent misses the prior state, test a bounded history view through the existing `get` operation before changing active search or adding a tool. Desktop-app testing is user-led only if a problem appears; CLI results do not establish app behavior.
