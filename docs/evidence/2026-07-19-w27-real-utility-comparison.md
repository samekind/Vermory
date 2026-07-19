# W27 Real Utility Comparison

## Result

W27 completed one direct real-provider lane over four frozen reality cases and
six context conditions. It compares memory systems, not models. The final lane
used direct SiliconFlow `deepseek-ai/DeepSeek-V4-Flash`; it did not use NewAPI,
a mock provider, copied fixture scores, or a Pro model.

The final native condition completed and passed all four downstream tasks with
zero stale, deleted, injected, cross-scope, or cross-tenant forbidden hits.
The best completed simple baseline passed one of four tasks. Vermory used 816
context bytes across the four cases, compared with 2,858 bytes for full
history.

| Condition | Completed | Successful | Forbidden hits | Context bytes |
| --- | ---: | ---: | ---: | ---: |
| `no_context` | 4/4 | 0 | 0 | 0 |
| `full_history` | 4/4 | 1 | 0 | 2,858 |
| `plain_summary` | 4/4 | 1 | 1 | 2,046 |
| `plain_retrieval` | 4/4 | 0 | 0 | 1,517 |
| `mem0_oss` | 4/4 | 1 | 0 | 5,586 |
| `vermory_native` | 4/4 | 4 | 0 | 816 |

This supports the frozen profile's positive utility claim: native is no worse
than the best completed simple baseline, is strictly more successful, has zero
forbidden hits, and uses less context than full history. It is not a universal
quality, latency, provider, or model-ranking claim.

## Frozen Contract

The final execution profile was `real-utility-comparison-v6`:

| Field | Value |
| --- | --- |
| Profile SHA-256 | `b3b92694e14ed2449da9d8e948429b676e18975ff8d9bcc414cfa4fcd38e6508` |
| Scorer | `utilityeval-checks-v3` |
| Workers | 2 |
| Thinking | disabled |
| Temperature | 0 |
| Context bundle SHA-256 | `5106ac23cd53235ea4aa15239a33661c78a556a0b2403bf5dfbf06e90a2ae95c` |

The bundle contains 4 cases x 6 conditions. It binds the complete profile
digest and includes declared scoring aliases. Bundle construction independently
verified each native receipt's case, delivery ID, body SHA-256, byte count,
retrieval mode, retrieval profile, tenant ownership, and exact PostgreSQL
delivery body. It did not reset or reseed the native database.

## Provider Evidence

The final real run produced 24 inputs, 24 outputs, 24 score artifacts, and 24
raw provider artifacts, with no provider failures and an empty stderr log.

| Artifact | SHA-256 |
| --- | --- |
| Model-run binary | `875b67e74089624cc6d381e2bfc72f4ed878ebc93ac75f331352dbe82ef05ef4` |
| `report.json` | `ec6258df477087c62de79e5358cc39eee36cb3467a003013e7ca521ff9f11d27` |
| `report.md` | `f28da70454b5a40c929319f79af8abde522dd4445808e6bbbbeba21b5a862c52` |

The report records the provider, model, profile digest, scorer, workers,
thinking mode, and temperature. Results are reassembled in frozen
case-and-condition order even though provider calls execute concurrently.

## Native Cases

| Case | Required behavior | Result |
| --- | --- | --- |
| `W01-synapseloom-continuity` | Current repository facts outrank stale workspace history | passed, zero forbidden hits |
| `C01-device-maintenance-continuity` | Corrected deletion, exact storage state, and QQ/WeChat boundary | passed, zero forbidden hits |
| `G01-language-default-local-override` | Expired task-local English rule does not pollute the Chinese global default | passed, zero forbidden hits |
| `S01-deletion-and-source-injection` | Deleted code remains unavailable while valid rotation guidance survives | passed, zero forbidden hits |

W01, C01, and S01 used direct SiliconFlow `BAAI/bge-m3` vector retrieval over
PostgreSQL-governed authority. G01 used the explicit Global Defaults path. C01
retained the `1,333,470` Gboard fact that the lexical native run had missed.

## Writeback And Isolation

After the model run, the four successful native outputs were read from their
retained local artifacts and checked against the report output hashes. The
writeback command rejected incomplete matrices, changed contexts, changed
outputs, unsuccessful native cells, undeclared lanes, and profile/bundle/report
digest mismatches before touching PostgreSQL.

The production service then attached each output to its exact delivery and
stored it as `agent_result`:

| Check | Result |
| --- | ---: |
| First writes | 4 |
| First writes already replayed | 0 |
| Lifecycle `proposed` | 4 |
| Lifecycle `active` | 0 |
| Search projection rows | 0 |
| Immediate idempotent replays | 4 |
| Distinct tenants | 4 |

Writeback evidence SHA-256:
`1637cb82935f4717640e9a20c67c1c0a38e7ed8cc924fea2d7d642d3e7b1457b`.
The corresponding writeback binary SHA-256 is
`0dccfa900e6c5395882304219ebcc3f91a78f7553f037d7b4cf1a03f30723c8d`.

The final report validator also recomputed every aggregate from the 24 call
results before allowing a replay. Its binary SHA-256 is
`bd5819507cdd419dc081cbc96cf88ebef622c29d94cc28d11e4d12a6cb801b4d`.
All four first calls in that second execution were recognized as replays, and
the database still contained exactly four writeback observations.

An independent PostgreSQL query matched all four delivery IDs to their four
expected tenants and found zero `ORCHID-7419` occurrences. Another query found
exactly four run writebacks, all proposed, with zero active memories and zero
search projections.

## Official mem0 Lane

The comparison used official mem0 source commit
`17836748d7afe0521516c6a73c6a256680f05527`, `mem0ai 2.0.12`, PGVector,
SiliconFlow `BAAI/bge-m3`, and 30 isolated seed records. mem0 had its own
database and collection and could not mutate Vermory authority.

The S01 mem0 context itself contained the deleted `ORCHID-7419` marker and an
untrusted instruction. In the final deterministic provider run the consumer
did not repeat them, so the final mem0 aggregate had zero forbidden output
hits; earlier retained runs did emit forbidden content. Input leakage and
consumer output behavior are reported separately.

The loopback mem0 service returned HTTP 200 before teardown. Its launchd job
was removed, port 8891 stopped listening, and the same check then returned no
HTTP response. Only after that teardown did Vermory complete the four native
PostgreSQL writebacks and their idempotent replays. This proves the native
writeback path remained usable without the mem0 process.

## Failure Ledger

No failed or weaker run was overwritten:

| Run | Retained result |
| --- | --- |
| Initial scorer v1 | 24/24 calls; native 2/4 because valid Chinese and `%` forms were rejected |
| First launchd wrapper | no model calls; `zsh` read-only variable error, job removed |
| Partial serial scorer-v2 attempt | partial artifacts only; stopped before the controller deadline, no report |
| scorer-v2, 4 workers | 13/24 completed; 11 explicit provider timeouts |
| v3, 2 workers, non-thinking | 24/24 completed; native 2/4, further wording false negatives |
| v4 aliases | 24/24 completed; native 3/4 |
| v5 scorer-v3 | 24/24 completed; native 2/4 under implicit provider sampling |
| v6 deterministic | 24/24 completed; native 4/4, zero native forbidden hits |

The scorer evolution is disclosed because it followed observed false
negatives. Scorer v3 uses declared, profile-bound RE2 predicates for bounded
surface variation. Positive lifecycle patterns require their subject and
relation; unit tests prove that negated deletion and rotation statements do not
match. Forbidden facts never receive semantic-similarity or LLM-judge waivers.

## Public Evidence Boundary

The committed JSON beside this document contains only normalized counts,
hashes, profile parameters, case results, and the failure ledger. Raw provider
responses, private absolute paths, runtime account details, credentials,
Keychain values, and private environment data remain outside Git. No credential
was placed in a command line, report, source fixture, or committed artifact.
