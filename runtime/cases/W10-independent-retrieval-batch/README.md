# W10 Independent Retrieval Batch

W10 is the second independent retrieval-quality batch for H-009. It uses a
different corpus composition and query wording from W08 while retaining the
same authority, lifecycle, tenant-isolation, active-only projection, and
rebuild-equivalence gates.

The batch covers four user-facing domains: software release work, thesis
research, home maintenance, and household purchasing. It includes Chinese,
English, mixed-language, exact identifier, path, date, duration, numeric,
multi-fact, and continuity-isolation queries. Every record points to an
existing public casebook source for provenance.

`forbidden_record_ids` is reserved for zero-tolerance violations: a different
scope or tenant, an ineligible lifecycle state, a deleted marker, or a fact
explicitly excluded by the task. Ordinary same-scope distractors remain
unforbidden so that ranking quality is measured by Recall, MRR, and nDCG
instead of incorrectly treating every extra candidate as a safety violation.

Corpus version 2 enforces that rule in validation. It removes the active
same-scope shopping-budget distractor from the order-date query's forbidden
set; the record and query remain unchanged, so a lower-ranked budget result
affects ranking quality without being misreported as isolation or lifecycle
failure.

The runner must seed the records through Vermory's authoritative runtime and
must use the direct SiliconFlow OpenAI-compatible embedding endpoint:

```text
base URL: https://api.siliconflow.cn/v1
model: BAAI/bge-m3
dimensions: 1024
```

The key is supplied through an environment variable and is not part of the
corpus or any committed artifact.

This batch is evidence for retrieval behavior on this frozen corpus. It does
not by itself accept a ranking algorithm, switch the production default,
qualify production scale, or establish source-authority ranking.
