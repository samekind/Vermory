# W12 Server Qualification Scale Profile

W12 calibrates one named server profile on fixed reference hardware. It is not
a universal production claim and it does not use synthetic records to claim
memory quality.

The profile creates 100,000 active governed facts across 10 tenants and 100
continuities, then creates 450,000 append-only governed revisions. The final
authority contains 100,000 active and 450,000 superseded facts. Initial active
writes plus revision activation and supersession produce exactly 1,000,000
durable projection events. It verifies that tenant lag is counted from actual
pending rows rather than global identity gaps. A current-authority snapshot
bootstrap builds 100,000 active lexical and vector projections without
replaying all historical events, then ordinary workers consume only the tail.

Fifty clients issue 1,000 scoped lexical and vector queries while 1,000 facts
are deleted. Hard gates require zero tenant or continuity leakage, zero deleted
residue, zero final lag, stable cursor monotonicity, and exact projection counts.
Latency, duration, and database size are calibrated targets tied to the case's
M4 Pro 48 GiB reference machine. A small direct SiliconFlow probe remains
separate from the deterministic scale vectors.

This case does not qualify HA, replication, PITR, cross-region delivery,
unbounded retention, one million vector rows, or memory-formation quality.
