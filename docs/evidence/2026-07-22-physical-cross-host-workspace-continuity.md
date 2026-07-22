# Physical Cross-Host Workspace Continuity Evidence

Date: 2026-07-23

Runtime case: `W34-physical-cross-host-workspace-continuity`

Reality case: `W05-trusted-workspace-attachment`

Status: `physical-runtime-qualified / Grok-generation-external-blocked`

Source revision: `ceafea623791beeeb1f2a10e70f3f45a0bae494c`

GitHub Actions run: [`29934847722`](https://github.com/samekind/Vermory/actions/runs/29934847722)

The normalized result is retained in the
[W34 machine snapshot](snapshots/2026-07-22-physical-cross-host-workspace-continuity.json).

## Boundary Executed

This run used two real physical hosts and two independent non-bare Git clones
of `samekind/Vermory`. Both clones were at the exact source revision and had
the same HTTPS remote, but their canonical roots, trusted filesystem
namespaces, Git common fingerprints, and attachment fingerprints differed.

The operator workstation clone was under the external volume. The target clone,
PostgreSQL 18.3, restricted MCP process, isolated Grok state, and signed release
binary were on the Mac mini. No local or remote `sudo`, local PostgreSQL data
directory, NewAPI route, database port forwarding, or credential transfer was
used.

The executed platform trajectory was:

```text
source continuity and current fact
-> unbound target abstention with zero side effects
-> independently governed distractor tenant at the target anchor
-> explicit rebind and idempotent replay
-> target-only governed recall over MCP
-> deterministic artifact and proposed-only writeback
-> exact writeback replay
-> explicit reversal
-> source restored and target abstention restored
```

## Exact Signed Release

The protected Draft PR head passed every exact-head CI job:

| Job | ID | Result |
|---|---:|---|
| PostgreSQL full suite, race, vet, integrations, manifest | `88973645778` | pass |
| Linux service lifecycle, AMD64 | `88973645579` | pass |
| Linux service lifecycle, ARM64 | `88973645465` | pass |
| DEB package, AMD64 | `88973645539` | pass |
| RPM package, AMD64 | `88973645507` | pass |
| DEB package, ARM64 | `88973645641` | pass |
| RPM package, ARM64 | `88973645585` | pass |
| OIDC signed snapshot | `88974697163` | pass |

Artifact `8535789948` was downloaded to the external volume. Its transport ZIP
SHA-256 exactly matched the GitHub artifact API digest:

```text
9bce20e9653c972a6617cf6917f91dd4826c2582f6e651ef530555e82e0b2b37
```

All 12 release-manifest payloads verified. Official Cosign `v3.0.6` independently
verified the manifest against:

```text
identity: https://github.com/samekind/Vermory/.github/workflows/ci.yml@refs/pull/1/merge
issuer:   https://token.actions.githubusercontent.com
```

Adding one newline to the manifest failed signature verification. Verifying the
unchanged manifest against `release.yml` failed the identity check. The
extracted Darwin ARM64 binary reported the exact revision and had SHA-256:

```text
48947a5cd587ea1d4724fce098f68c7dbff5b238f5f5a6b68b6bd40a3a8bbdcf
```

The same binary bytes were measured on the workstation and Mac mini before the
new W34 database was migrated to schema `24`.

## Anchor And Abstention Evidence

The source attachment used namespace `w34-workstation-source`; the target used
`w34-macmini-target`. Their attachment fingerprints were:

```text
source: 46318d678c2424740c0f677f7fbd92e67a0e3c073f63210b27a05922512825d5
target: eb3b36fdb875ed14d8dcbf284bd7969658ae964cdfa8df294f83d494a3d19b7c
```

Matching repository remote and commit did not authorize attachment. Before
rebind, target `prepare_context` returned:

```json
{"status":"needs_confirmation","delivery_id":"","context":""}
```

The target tenant ledger was `0 delivery / 1 observation / 1 memory` both
before and after that call. The one observation and memory were the explicitly
seeded source fact, so target abstention created no side effect.

The real MCP handshake negotiated protocol `2025-06-18`, advertised exactly
`prepare_context` and `commit_observation`, and produced model-visible tool
schema SHA-256
`9c0ac55ce9efd60979707c6269d1ae84dd15e22837d440887763b93d6cff27ec`.
The schemas exposed no tenant, repository-root, filesystem-namespace, binding,
adopt, rebind, or reversal authority.

## Rebind, Isolation, And Writeback

The explicit operator rebind created bridge
`264a7288-7f9a-410c-ab60-d56b54257ab3` and preserved continuity
`306f5c58-3dc2-4b76-9c2e-9897700a38d3`. Replaying the same operation returned
the same bridge ID with `replayed=true` and no second binding change.

After rebind:

- the source binding was `retired`;
- the target binding was `confirmed` on the same continuity;
- the other tenant remained `confirmed` on independent continuity
  `db1f4a84-e859-40c1-bbd2-bea88345d42e`;
- the target delivery contained only `continuation_marker=w34-current`;
- the other-tenant delivery contained only
  `continuation_marker=w34-other-tenant-only`.

An independent MCP SDK consumer then created the frozen target artifact. Its
content was one newline-terminated line and SHA-256:

```text
continuation_marker=w34-current
ede4f57a80f003593489ed55de503804ff17f65bc1c5ca671ee6b468c20bc21b
```

The first writeback returned `memory_status=proposed` and `replayed=false`.
The exact replay returned the same observation
`a1692de6-f4b7-476c-9564-6fab3921b361`, the same proposed memory
`3f80707c-1af7-450f-8ce7-9afbd1cc660d`, and `replayed=true`. PostgreSQL contained
one active target fact, one proposed writeback, one active other-tenant fact,
and zero search projection rows for the proposed memory.

This SDK result qualifies the platform runtime boundary. It is not attributed
to Grok.

## Reversal

The explicit reverse operation changed the bridge to `reversed`. The exact
post-reversal state was:

| Boundary | Result |
|---|---|
| target-tenant source binding | `confirmed` |
| target-tenant target binding | `retired` |
| other-tenant target binding | `confirmed` |
| source preparation | `resolved`, current fact present |
| target preparation | `needs_confirmation`, empty delivery and context |
| target post-reversal delivery rows | `0` |
| current governed memory | still `active` |
| SDK writeback | still `proposed` |
| other-tenant fact | unchanged and `active` |

## Real Grok Boundary

The isolated Grok state contained exactly one W34 MCP server. `grok mcp doctor
--json` reported a healthy process, protocol `2025-06-18`, and two tools. Its
remote authentication source simultaneously reported `auth expired`.

A fresh coding run was still attempted with a prompt that did not contain the
marker value. Grok exited before model generation with:

```text
Not signed in. To authenticate without a browser, run:
  grok login --device-code
```

The failed run created no artifact, delivery, observation, or memory. The SDK
trajectory was executed only after this zero-side-effect check and is recorded
under a distinct client identity and operation IDs.

## Acceptance

| Layer | Result |
|---|---|
| signed exact release | pass |
| two physical hosts and two real clones | pass |
| conservative attachment and zero-side-effect abstention | pass |
| explicit rebind, replay, isolation, and audit ledger | pass |
| production MCP protocol and schema | pass |
| deterministic SDK artifact, proposed writeback, and replay | pass |
| exact reversal | pass |
| fresh authenticated Grok `prepare_context` and context receipt | external blocked |
| Grok-authored artifact and `commit_observation` | external blocked |
| W34 hard gates | `28 / 32` passed; `4` external blocked; `0` platform failures |

W34 is not declared `32/32` and is not a Grok-client qualification. A later
run must refresh Grok authentication and repeat the full client trajectory on a
fresh operation set; an SDK, Codex, Cursor, mock, or prior Grok transcript
cannot replace those four gates.

## Claim Boundary

This evidence qualifies exact signed-binary deployment, physical cross-host
workspace rebind, conservative anchor handling, tenant isolation, MCP runtime
behavior, proposed-only client writeback, idempotent replay, and reversible
governance for this exact case.

It does not qualify Grok generation, Cursor generation, automatic clone
identity merging, transfer of uncommitted workspace state, PostgreSQL
cross-host replication, long-duration network reliability, or the complete
Vermory platform.
