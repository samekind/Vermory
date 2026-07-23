# W11 Projection Outbox Fault Profile

W11 qualifies the PostgreSQL projection event stream as a transactional outbox
for the current self-hosted profile. It uses a disposable PostgreSQL 18 cluster
and does not touch the developer's shared database.

The profile creates 1,000 governed active facts through the authoritative
observation/governance transaction, then verifies bounded backlog processing,
provider failure retry, at-least-once replay, immediate PostgreSQL restart
during embedding work, same-pool recovery, and concurrent deletion. A separate
tenant performs a final projection and vector query through the direct
SiliconFlow `BAAI/bge-m3` endpoint after the restart.

This case measures outbox correctness and recovery. It is not a 100k/1M scale
qualification, an HA claim, or a queue throughput SLO.
