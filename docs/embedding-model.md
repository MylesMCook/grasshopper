# Embedding model decision

Grasshopper 2.4.0 uses IBM Granite embedding small English R2 through the
unmodified FP32 ONNX Community export at revision
`1dc7835ba0cb9c76a3618d0bf0c427c97671b3c8`.
[IBM model card](https://huggingface.co/ibm-granite/granite-embedding-small-english-r2)
and [export](https://huggingface.co/onnx-community/granite-embedding-small-english-r2-ONNX/tree/1dc7835ba0cb9c76a3618d0bf0c427c97671b3c8).
The graph, adjacent external weights, and tokenizer each have a pinned SHA-256
digest. The Apache 2.0 license and source notice ship in server archives.

## Why this model

Twelve model configurations were compared using the unchanged SQLite search
pipeline on two separate synthetic corpora with 160 labeled queries. On a Mac
mini M4 Pro with ONNX Runtime 1.30.0:

| Model | Hybrid top 1 | Hybrid top 3 | Median query embedding | Model weights |
| --- | ---: | ---: | ---: | ---: |
| BGE small English v1.5 | 110/160 | 135/160 | 3.10 ms | 127 MiB |
| Granite small English R2 | 122/160 | 143/160 | 3.23 ms | 186 MiB |
| Arctic XS uint8 | 111/160 | 134/160 | 1.36 ms | 22 MiB |

Granite improved first-place retrieval on 15 queries and regressed on three.
The eight longer handoff queries improved from 2/8 to 7/8 first-place and from
3/8 to 8/8 top-three. Excluding those longer cases, the gain was 7/152.
These are exploratory, synthetic results, not a claim of universal accuracy.
The second corpus alone slightly favored MiniLM. Search results still require
inspection; none of the models reliably identified an absent answer.

The frozen corpora are in `tests/fixtures/embedding-search-eval.json` and run
against separate synthetic databases. Production-adapter tests enforce the
observed Granite recall, the export's published cosine examples, current scope
and archive boundaries, immediate writes, and re-embedding/reopen behavior.
The existing hybrid weights and similarity cutoff remain unchanged.

## Runtime bound

Granite uses prefix-free query/document encoding and its `sentence_embedding`
output, normalized to 384 dimensions. The export supports 8192 tokens, but a
maximum-window CPU inference exceeded the service's five-second deadline on
the Mac mini; maximum-size writes at 2048 also exceeded that deadline on
smaller Mac and Windows CI runners. Grasshopper caps tokenization at 1024,
compared with BGE's 512.
This bound preserves every measured fixture result. The complete memory text
remains stored and returned by `get`; only embedding input is truncated.
Native tests also store and read back a maximum-size 32768-byte memory through
the authenticated MCP service under the existing inference deadline.

Inference uses at most four CPU threads and sleeping idle workers. Pools using
every CPU core with ONNX's default spinning intermittently timed out even on a
short Windows write while other native test processes were running. A Mac
four-process probe reduced worst observed maximum-size embedding latency from
1158 ms to 240 ms with the bounded, non-spinning pool. This is a contention
check, not a general latency guarantee.

## Upgrade and rollback

Equal vector dimensions do not make the model spaces compatible. The server
rejects old vectors. Stop the sole writer, keep a consistent backup, and use
the new migration binary to re-embed a separate database. Verify current and
historical records, scopes, credentials, and recall before replacing the live
database. See [re-embedding](operations.md#re-embed-when-upgrading-from-23x-or-earlier).

Retain the old archive, configuration, and BGE database for rollback. If writes
were accepted after cutover, preserve the new database before rolling back and
reconcile those writes. Never replace a newer database silently or run two
writers. Real-machine results belong in [verification](verification.md).
