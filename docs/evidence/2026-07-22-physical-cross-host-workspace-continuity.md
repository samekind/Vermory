# Physical Cross-Host Workspace Continuity Evidence

Date: 2026-07-23

Runtime case: `W34-physical-cross-host-workspace-continuity`

Reality case: `W05-trusted-workspace-attachment`

Status: `physical-cross-host-grok-client-qualified`

Source revision: `2f67366b0b6afb29393dd96bc50aebbf044b61d5`

GitHub Actions run:
[`29991498524`](https://github.com/samekind/Vermory/actions/runs/29991498524)

The normalized result is retained in the
[W34 machine snapshot](snapshots/2026-07-22-physical-cross-host-workspace-continuity.json).
Raw credentials and raw Grok traces remain outside the repository.

## Boundary Executed

This run used two physical hosts and two independent non-bare Git clones of
`samekind/Vermory`. Both clones were at the exact source revision and had the
same HTTPS remote. Their canonical roots, trusted filesystem namespaces, Git
common fingerprints, and attachment fingerprints were different, so matching
Git identity did not authorize continuity.

The accepted trajectory was:

```text
source continuity and current governed fact
-> unbound target abstention with zero side effects
-> separately governed distractor tenant at the target anchor
-> explicit rebind and exact replay
-> real Grok prepare_context over MCP
-> Grok writes the deterministic artifact
-> Grok commit_observation remains proposed
-> independent SDK exact replay and tenant-isolation check
-> explicit reversal
-> source restored and target abstention restored
```

The workstation clone, GitHub artifact, Go caches, helper binary, and execution
evidence were kept on the external volume. The target clone, PostgreSQL
database, exact Vermory binary, and isolated Grok state were on the Mac mini.
No `sudo`, NewAPI route, database port forwarding, or credential material in
public evidence was used.

## Exact Signed Release

Pull request #2 was open at the exact source revision and all 18 CI jobs passed,
including PostgreSQL tests, race tests, release construction, Linux service and
package lifecycle jobs on AMD64 and ARM64, repository lifecycle jobs, and the
OIDC signing job.

The raw GitHub artifact had 66,675,000 bytes. Its independently measured ZIP
SHA-256 exactly matched the GitHub artifact API digest:

```text
2b1115a5a316cab461d728527a4d2903660ada499118c39125941ed58b3affb4
```

All 16 release-manifest entries and all 8 platform checksums verified.
Official Cosign `v3.0.6` verified the manifest against:

```text
identity: https://github.com/samekind/Vermory/.github/workflows/ci.yml@refs/pull/2/merge
issuer:   https://token.actions.githubusercontent.com
```

Using another artifact file with the bundle failed signature verification.
Using the correct manifest with the pull request #1 identity also failed. The
Darwin ARM64 binary reported the exact revision and had SHA-256:

```text
5328479583111894b14752a133ff969c84f5b75775bd60239e1260e424e85da0
```

The exact same binary bytes were measured on both hosts before schema `25` was
created and compatibility-checked with a dedicated runtime role.

## Anchor, Abstention, And Rebind

The source attachment used namespace `w34-workstation-source`; the target used
`w34-macmini-target`. Their attachment fingerprints were:

```text
source: 013c78904d79f9f1691b6a974f57430a7b2ba3eade42668c0333b422c8b2951d
target: 44889dd54036206c4c7ced5da631cdfe1f429c0910c7019c53b17137b4bf1d64
```

Before governance, target `prepare_context` returned:

```json
{"status":"needs_confirmation","delivery_id":"","context":""}
```

The target tenant remained at `0 delivery / 1 observation / 1 memory`; the
observation and memory were the explicitly seeded source fact. The real MCP
handshake negotiated protocol `2025-06-18`, advertised exactly
`prepare_context` and `commit_observation`, and produced tool-schema SHA-256:

```text
9c0ac55ce9efd60979707c6269d1ae84dd15e22837d440887763b93d6cff27ec
```

The schemas exposed no tenant selection, repository path, namespace, binding,
adopt, rebind, or reversal authority.

The accepted explicit rebind created bridge
`208048ff-8d57-4722-a2a0-0a0ba91304be` and preserved continuity
`07b01687-6638-4d61-b46a-9378b4e635d1`. Replaying the same operation returned
the same bridge ID with `replayed=true`. The source binding became `retired`,
the target binding became `confirmed`, and the other tenant retained its own
confirmed binding and independent continuity.

## Real Grok Client

The Mac mini used official Grok CLI `0.2.101` with a fresh isolated state,
disabled memory, plans, subagents, web access, and shell commands, and one
registered W34 MCP server. `grok mcp doctor --json` reported one healthy server,
protocol `2025-06-18`, and two tools.

The target network could not reach the provider directly. A temporary
loopback-only reverse proxy carried only this run's provider traffic through
the workstation's working proxy. A no-credential request returned `401` before
the model run, proving the route reached the provider. The primary
`grok-build` route reported a spending-limit `402`; the official CLI then used
its `grok-4.5-build-free` fallback. This evidence qualifies that actual fallback
route, not the paid route and not model quality.

The accepted prompt did not contain `w34-current`. The official session trace
records:

| Event | Timestamp |
|---|---|
| `prepare_context` completed | `2026-07-23T11:50:10.229Z` |
| artifact edit completed | `2026-07-23T11:50:13.384Z` |
| `commit_observation` started | `2026-07-23T11:50:15.354Z` |
| `commit_observation` completed | `2026-07-23T11:50:15.368Z` |

The trace therefore proves the required sequence rather than inferring it from
the final text. `prepare_context` returned only:

```text
continuation_marker=w34-current
```

Grok created a one-line, newline-terminated
`w34-continuity-report.txt`, then called `commit_observation` with delivery
`df6fdcdb-ad4e-4658-83ca-50c30ed9fbf1`. The first writeback returned:

```text
observation: 6a342176-f6bd-4202-8aa5-dbe2f5b3c4dc
memory:      c79651fc-870a-4b15-8bce-dc5cdcfad82f
status:      proposed
replayed:    false
```

The Grok run made five model calls and ended with `W34_COMPLETE`. Its retained
trace, session export, and streaming output had SHA-256:

```text
trace:   2f04c5d8bcce894f00f0c1dc8807b47fe6d3d8157f4fcb4070ed54d5db9a360a
session: a16b024eb08b84765060d9b517f6e6e9e4925b7334ba8ceb9444cbb312285bd4
stream:  19427067c1b9997acfbc589c64b9afba0ce85b0807be3c33d39d66845ca3c2b3
```

## Artifact, Replay, And Isolation

An independent Go MCP SDK replayed the exact Grok prepare and commit
operation IDs. It received the same delivery, observation, and proposed memory,
with `replayed=true`. It independently read the target artifact and measured:

```text
continuation_marker=w34-current
ede4f57a80f003593489ed55de503804ff17f65bc1c5ca671ee6b468c20bc21b
```

The other tenant received only
`continuation_marker=w34-other-tenant-only`. PostgreSQL contained:

- one active current fact for the target tenant;
- one proposed memory from the accepted Grok run;
- one active distractor fact for the other tenant;
- zero projection rows for proposed memory;
- one accepted target delivery and one accepted other-tenant delivery;
- the exact Grok observation and delivery IDs recorded by the trace.

One earlier non-qualifying Grok attempt also remains as a proposed-only,
unprojected observation because it committed before its file edit completed.
It is not used as accepted evidence and was not deleted to make the ledger look
clean.

## Reversal

The accepted bridge was explicitly reversed. The final state was:

| Boundary | Result |
|---|---|
| target-tenant source binding | `confirmed` |
| target-tenant target binding | `retired` |
| other-tenant target binding | `confirmed` |
| source preparation | `resolved`, current fact present |
| target preparation | `needs_confirmation`, empty delivery and context |
| target post-reversal delivery rows | `0` |
| current governed memory | `active` |
| accepted Grok writeback | `proposed`, zero projection rows |
| other-tenant fact | unchanged and `active` |

## Retained Failures

Failures observed before the accepted run remain part of the evidence:

- an expired refresh token was rejected before model generation;
- the first new attempt used an invalid Grok tool allowlist and never created a
  session;
- the second attempt stalled on the target provider route and was terminated
  with zero MCP or database side effects;
- the third attempt completed but committed before its artifact edit, so it was
  rejected as the accepted trajectory and its proposed-only writeback retained;
- the first remote HTTPS clone disconnected, so a source bundle was transferred
  and the target was recreated with `git clone`, then normalized to the same
  HTTPS remote and exact commit.

## Acceptance

| Layer | Result |
|---|---|
| signed exact release | pass |
| two physical hosts and two real clones | pass |
| conservative attachment and zero-side-effect abstention | pass |
| explicit rebind, replay, isolation, and bridge ledger | pass |
| production MCP protocol and schema | pass |
| real Grok prepare and governed context receipt | pass |
| sequential Grok artifact then proposed writeback | pass |
| independent artifact hash and exact replay | pass |
| PostgreSQL authority and tenant isolation | pass |
| exact reversal | pass |
| W34 hard gates | **`32 / 32` passed** |

## Claim Boundary

This evidence qualifies one exact signed-binary, two-physical-host workspace
rebind through a real Grok MCP consumer using the official free fallback:
conservative anchor handling, current-only context, deterministic artifact,
proposed-only writeback, exact replay, tenant isolation, and reversible
governance.

It does not qualify the paid Grok route, model ranking, Cursor generation,
automatic repository-intent inference, movement of uncommitted state,
PostgreSQL replication, high availability, long-duration network reliability,
or the complete Vermory platform.
