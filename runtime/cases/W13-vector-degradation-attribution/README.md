# W13 Vector Degradation Attribution

W13 explains the controlled vector fallbacks preserved by W12. The original
W12 run issued 550 vector requests while 1,000 deletes created projection tail
events. It recorded 138 effective vector results and 412 degraded results, but
the first payload did not include failure-code counts.

Two controls separate the cause:

- a 100,000-vector run with no concurrent authority changes completed all 550
  requests as effective vector results with zero degradation;
- an instrumented replay of the W12 delete/tail schedule recorded 145 effective
  vector results, 405 `projection_lag` fallbacks, and zero other degraded
  results.

The exact split between effective and lagged requests is intentionally
scheduler-dependent. The invariant is:

```text
effective vector + projection_lag fallback = 550 requested vector queries
other degraded queries = 0
```

This case rejects the initial scoped-HNSW hypothesis. It validates Vermory's
projection-current gate and exact lexical fallback during concurrent authority
change. It does not change the lexical default, vector query algorithm, or
pgvector settings.

Frozen case SHA-256:
`ae00edce919813287c21ff06f53a8d104b911254a4cb34c5648e57eeb0235125`.
