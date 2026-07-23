# Operational Scale Profile Evidence

Date: 2026-07-15

Status: completed as an opt-in PostgreSQL operational profile

## Execution

The profile uses synthetic records only for database, indexing, concurrency,
deletion, and connection-recovery measurement. It does not claim memory quality
or replace the real W08/W10 retrieval cases.

Command:

```bash
VERMORY_SCALE_PROFILE=1 \
VERMORY_SCALE_DATABASE_URL='postgresql:///vermory_scale?host=/tmp' \
go test -p 1 -count=1 -v ./internal/runtime \
  -run '^TestOperationalScaleProfile$'
```

The run used the current committed runtime and PostgreSQL schema 15. Authority
records were created through the runtime's observation and governed-memory
transaction path, not by inserting rows into a search projection.

## Profile

| Measurement | Result |
|---|---:|
| active governed memories seeded | 10,000 |
| concurrent readers | 8 |
| queries per reader | 25 |
| concurrent deletions | 50 |
| active count after seed | 10,000 |
| deleted count after concurrent writes | 50 |
| seed duration | 63.634 s |
| search p50 | 17 ms |
| search p95 | 210 ms |
| connection recovery after `pg_terminate_backend` | pass |
| post-recovery query | pass |

The workload mixes English service identifiers, HTTP paths, flags, numeric
retry budgets, durations, and Chinese content. Search and deletion ran
concurrently. After deletion, exact probes for deleted memory IDs returned no
active result. A PostgreSQL backend connection was terminated while the pool
was live; the pool recovered within the bounded retry window and served a new
query.

## Interpretation

The current PostgreSQL-native design has a measured 10k operational profile on
this machine and preserves authority/deletion behavior under concurrent reads
and writes. The result supports continuing with PostgreSQL as the default
operational stack for calibrated self-hosted deployments.

It does not establish a 100k active-memory or 1M projection SLO. Larger scale,
long-running backlog behavior, restart during provider work, and deployment
hardware profiles remain separate qualification work. Real provider outage and
lexical fallback are covered by the production retrieval runtime evidence.
