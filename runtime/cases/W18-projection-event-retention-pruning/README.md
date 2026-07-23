# W18 Projection Event Retention And Pruning

This case qualifies bounded retention of `memory_projection_events` without
weakening PostgreSQL authority, deletion, tenant isolation, profile isolation,
or retrieval fallback behavior.

The 24 write epochs are an accelerated workload used to produce a repeatable
long-running event shape. They are not represented as 24 months, or any other
amount, of uninterrupted wall-clock uptime.

The formal run establishes two existing projection subscribers, intentionally
lags one subscriber, prunes only through the safe cursor/cutoff/tail bound,
injects a PostgreSQL restart before prune commit, and then proves that a future
subscriber and a reset subscriber require an authority snapshot rebuild. One
direct SiliconFlow `BAAI/bge-m3` projection/query probe follows the mechanical
qualification.

`memory_projection_events` remains a disposable delivery log. This case does
not delete governed memory or replace authorized forgetting.
