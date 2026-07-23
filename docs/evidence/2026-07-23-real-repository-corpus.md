# Real Repository Corpus Qualification

Date: 2026-07-23

Reality case: `W05-trusted-workspace-attachment`

Runtime case: `W37-real-repository-corpus`

Status: `runtime-qualified`

Evidence level: `public`

## Question Tested

W31, W33, and W34 already exercise difficult local, remote-shaped, and
physical cross-host Git topology. W37 asks a different question: does the same
trusted attachment and PostgreSQL continuity path work on heterogeneous real
repositories, or only on repositories created by the Vermory tests?

The accepted run used six public HTTPS checkouts at exact commits:

| Repository | Shape | Exact commit | Nested probe |
|---|---|---|---|
| `golang/example` | Go multi-example repository | `7f05d217867b2af52b0a28c6d1c91df97e1b5b39` | `hello` |
| `rust-lang/mdBook` | Rust workspace | `4f8c9460977e18974b37bb9a2292219bfa317632` | `crates` |
| `django/django` | Python framework repository | `21b3a4c4c528ca827adda14b6c4cf045afba6d4f` | `django` |
| `pnpm/pnpm` | Node.js monorepo | `50ed6f5070fa5b159f4cfd56e6de8ffddeae4d3e` | `pnpm` |
| `mem0ai/mem0` | Memory-platform monorepo | `ca2abca2b884e038d3e525070e79d3057ef2012c` | `mem0` |
| `openclaw/openclaw` | Agent-platform monorepo | `9ca500db98574d1d0e87bdc412293d5196f2a8d8` | `src` |

These are public, read-only qualification inputs. No repository content is
committed into Vermory evidence.

## Exact Runtime

The accepted run was bound to implementation revision:

```text
ee4d30596e47acaa5b4a10965c4a22f47084b72a
```

Runtime versions:

```text
Git             2.55.0
PostgreSQL      18.4 Homebrew
Go              1.26.5 darwin/arm64
schema replay   0 -> 25
```

The checkouts, PostgreSQL data, logs, Go caches, temporary files, and raw
evidence were held on the external volume. The six sparse checkouts occupied
249 MiB and the W37 cluster/evidence root occupied 78 MiB at measurement time.
The system Go build and module caches both remained at 0 bytes. No `sudo`,
NewAPI route, LLM, embedding model, or optional memory backend was used.

## Accepted Trajectory

For each exact checkout, the runner performed both a repository-root probe and
a real nested-directory probe. All twelve probes returned the expected
canonical checkout root. Root and nested probes retained the same Git
common-directory fingerprint while producing different bounded attachment
receipts because their cwd values differed. The six repositories had six
distinct Git common-directory fingerprints, and every nested receipt survived
exact attachment encode/decode.

The fresh PostgreSQL authority then executed:

```text
six exact namespaced anchors
-> six distinct workspace continuities
-> one governed marker per continuity
-> one current-only prepare per repository
-> exact prepare replay
-> alternate namespace abstention
-> same anchor under another tenant
-> independent other-tenant marker and delivery
```

The runner passed all `18 / 18` frozen gates.

## Independent Ledger

An independent SQL query against the accepted database, after the Go runner
completed, returned:

```text
target active workspace continuities      6
target distinct confirmed continuities    6
target confirmed bindings                 6
target active governed memories           6
target deliveries                         7
prepare operation duplicate rows          0
missing expected markers                  0
cross-repository marker hits              0
alternate-namespace deliveries            0
other-tenant active continuities           1
other-tenant active memories               1
other-tenant deliveries                    1
other-tenant target-marker hits            0
target other-tenant-marker hits            0
```

The seventh target delivery is the separately named post-other-tenant
isolation probe. Exact prepare replay reused the original delivery rather than
creating another row.

## Integrity

Frozen inputs and runner:

```text
case.json    b9fe96bebf62d6f379d54f69e53e740c87ccc252dd1c6056fca9c4660a34c2f6
design       121992825d583b9fa41d4b43b034cce6d6b6aaaeac4c08a48be33a655bf87b21
Go runner    7f204ee2fbf524470b113c37bd11c55a89de6a3f19d1415d65441aac90be925d
```

External raw evidence:

```text
go-test.jsonl  301c6b50e7688ddc0e10ab263cd0f8d02a19c31e3481bc120ff69962fe9935ae
ledger.json    79abddabf8881f70567a763f25081189e147a573133b58680a3a841ae8a7e242
```

The raw-evidence privacy scan found zero personal absolute paths, database
URLs, email addresses, private-key markers, or API-key-shaped strings. The
public snapshot contains only normalized counts and public source identities.

## Retained Preparation Failures

The first nested-cwd attempt did not reach Vermory. The repositories had been
cloned with `--no-checkout`; setting sparse-checkout patterns alone did not
materialize directories under the installed Git version. The execution layer
therefore rejected nonexistent nested workdirs. Explicitly checking out the
frozen HEAD materialized only the selected sparse paths, after which all twelve
probes passed. This is retained as corpus-preparation failure, not reported as
a resolver failure.

## Claim Boundary

W37 qualifies the trusted attachment, exact namespaced resolution, governed
delivery, replay, cross-repository isolation, namespace abstention, and tenant
isolation contract for these six exact public repositories. It does not prove
every repository, Git implementation, filesystem, submodule layout, coding
client, or network condition.

The run does not change identity policy: remote URL, repository name, shared
history, language, or framework still cannot automatically merge
continuities. Unknown roots continue to require explicit confirmation, adopt,
or rebind. W37 is runtime evidence, not a model-quality, coding-client, public
hosting, or long-duration reliability claim.
