# W17 Active-Backlog Dimensional Migration

W17 qualifies coexistence of the active 1024-dimensional retrieval projection
and a candidate half-precision 2560-dimensional projection while PostgreSQL
authority events continue arriving.

The case creates 20,000 initial active governed facts across four tenants,
starts the candidate snapshot, commits 2,000 revisions, 500 deletions, and 500
new facts during the snapshot, and drains both profile-specific tails. It
injects a PostgreSQL immediate restart during candidate embedding, then proves
same-pool recovery, deletion safety, candidate reset/rebuild isolation, and
zero final lag for both physical projection classes.

Deterministic 1024- and 2560-dimensional embeddings qualify storage, worker,
cursor, lifecycle, restart, and scope mechanics. A separate small tenant uses
the direct SiliconFlow OpenAI-compatible embeddings endpoint with
`Qwen/Qwen3-Embedding-4B` and must return exactly 2560 dimensions before the
formal profile is accepted.

The original candidate `BAAI/bge-small-zh-v1.5` is retained as a failed
preflight result: the direct endpoint returned HTTP 400, provider error 20012,
`Model does not exist. Please check it carefully.` The same account returned
1024 dimensions for `Qwen/Qwen3-Embedding-0.6B`, 2560 for
`Qwen/Qwen3-Embedding-4B`, and 4096 for `Qwen/Qwen3-Embedding-8B`. W17 freezes
the available 4B/2560 tuple because it is dimensionally distinct from the
1024-dimensional incumbent and materially cheaper to qualify than 4096.

pgvector 0.8.5 rejects HNSW indexes on `vector` columns above 2000 dimensions.
The candidate therefore uses the supported `halfvec(2560)` storage type and
`halfvec_cosine_ops` HNSW index. This preserves all 2560 model dimensions while
making the physical precision change explicit in the `halfvec_2560` class.

This case does not rank embedding models, promote a semantic profile, claim
long-duration retention, or qualify cross-host HA.
