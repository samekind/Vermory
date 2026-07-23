# Production Retrieval Ablation

- Run: `w10-siliconflow-bge-m3-20260715-v5`
- Corpus SHA-256: `6a615f06a0598556b566e10bb089d506282990cceaedf91c8188ae6bbbe40f37`
- Implementation: `0526ef34f243ad330654b6fb6b65a318c182ad82`
- Engine: `rrf-v1`
- PostgreSQL schema: `15`
- Embedding: `BAAI/bge-m3` / `1024` dimensions
- Embedding requests: `102`
- Hard gates: PASS
- Projection rebuild equivalent: `true`
- Qualification: `measured`

## Conditions

| Condition | Queries | Hit@1 | Recall@K | MRR | nDCG@K | Forbidden | Ineligible | P95 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| `lexical_runtime` | 18 | 0.7222 | 0.7593 | 0.7500 | 0.7353 | 0 | 0 | 3.893958ms |
| `vector_pg` | 18 | 1.0000 | 1.0000 | 1.0000 | 0.9919 | 0 | 0 | 154.697334ms |
| `hybrid_rrf` | 18 | 1.0000 | 1.0000 | 1.0000 | 0.9908 | 0 | 0 | 154.952584ms |

## Non-Claims

- not a production default switch
- not a scale qualification
- not a sealed result
- not a source-authority ranking result
