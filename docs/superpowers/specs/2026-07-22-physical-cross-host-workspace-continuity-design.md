# Physical Cross-Host Workspace Continuity Design

Status: frozen for execution

Date: 2026-07-22

## 1. Purpose

This slice qualifies the remaining physical-host and second-client boundary of
`W05-trusted-workspace-attachment`. A real Git checkout on the operator
workstation is replaced by a separate clone on the Mac mini. Both clones have
the same upstream and commit, but their path and trusted filesystem namespace
are different. Vermory must abstain on the target until an operator explicitly
rebinds the existing continuity.

After rebind, a fresh authenticated Grok CLI coding run must consume current
governed context over the production MCP stdio boundary, create a deterministic
artifact in the target clone, and write one proposed observation. The frozen
runtime case is
`runtime/cases/W34-physical-cross-host-workspace-continuity`.

## 2. Boundary Under Test

The accepted trajectory is:

```text
external-volume source clone on operator workstation
-> trusted source attachment and governed continuity
-> independent target clone on Mac mini at the same commit
-> distinct target attachment returns needs_confirmation
-> explicit cross-namespace rebind
-> original continuity ID becomes reachable only from the target anchor
-> real Grok MCP prepare_context
-> deterministic artifact
-> commit_observation remains proposed
-> exact replay is idempotent
-> explicit reversal restores the source and removes target access
```

PostgreSQL and the Vermory MCP process remain on the Mac mini. This is a
workspace-continuity migration, not database replication or application-state
migration between PostgreSQL servers.

## 3. Storage And Host Discipline

All workstation-side clone, build cache, temporary data, downloaded release
artifacts, and execution evidence must be under `/Volumes/JSData`. The normal
local data volume and Homebrew PostgreSQL directories are not used. No local or
remote `sudo` is permitted.

Mac mini resources use a dedicated `w34` root, database, roles, tenant IDs,
filesystem namespace, MCP registration, Grok state, and ports. Existing
Vermory evidence services, databases, LaunchAgents, and Grok configuration are
read-only. Cleanup may remove only resources created for W34.

## 4. Git And Attachment Contract

Both checkouts must be created with the installed `git clone` command and must
be non-bare repositories at the same exact commit. Matching commit, remote URL,
basename, files, and repository history are diagnostics only. They do not grant
continuity.

The source attachment is generated from the source clone using
`w34-workstation-source`. The target attachment is generated on the Mac mini
from the target clone using `w34-macmini-target`. The roots, namespace IDs, and
attachment fingerprints must differ.

Before governance, preparing context for the target must return
`needs_confirmation`, an empty delivery ID, and empty context. The database
must show no target delivery, observation, or memory side effect.

## 5. Governance And Isolation Contract

The source binding and current fact are seeded through the operator CLI. A
second tenant is separately confirmed at the same target attachment and given
a distinct distractor fact. PostgreSQL remains the only semantic authority.

Migration uses the existing `bridge rebind` command with the exact source and
target roots and namespaces. The model cannot invoke this operation. The
accepted rebind must:

- retain the source continuity ID;
- retire the target tenant's source binding;
- confirm its target binding;
- leave the other tenant's target binding and fact unchanged;
- retain one active, inspectable bridge ledger row;
- replay idempotently for the same operation ID.

Reversal must retire the target binding and restore the exact source binding.
The governed current fact remains active. A fresh source preparation must then
resolve, while a fresh target preparation must abstain without a delivery.

## 6. Exact Binary And Database Profile

The Mac mini uses the Darwin ARM64 binary extracted from the protected signed
pull-request snapshot for the selected execution head. The binary payload
checksum, release revision, manifest signature identity, and schema
compatibility preflight must be verified before data mutation.

The W34 database is newly created and migrated to the exact schema supported by
that binary. A restricted runtime role is provisioned after migration. Runtime
MCP access uses that role; operator seed, rebind, reversal, and evidence queries
use the dedicated administrator role. Existing Mac mini databases are not
reused.

## 7. Real Grok Client Contract

W34 uses the installed official Grok CLI on the Mac mini with a new mode-`0700`
home and state directory. Existing login material may be copied only within the
same host, with mode `0600`; it is never printed, transferred to the
workstation, committed, or included in evidence hashes.

The isolated client has memory, plans, subagents, and web search disabled. It
registers exactly one W34 MCP server. `grok mcp doctor --json` must negotiate
protocol `2025-06-18` and discover exactly these tools:

```text
prepare_context
commit_observation
```

The model-visible schemas expose task and delivery workflow fields only. They
must not expose tenant selection, repository path selection, confirmation,
adopt, rebind, reverse, or memory activation.

The coding task supplies no fact value. Grok must call `prepare_context`, stop
if the status is not `resolved`, and otherwise write the exact frozen fact to
`w34-continuity-report.txt`. It then calls `commit_observation` with the
delivery ID and a bounded result statement. The writeback must remain
`proposed`. An exact replay must return the same observation and
`replayed=true`.

If authentication, account state, provider availability, or MCP discovery
fails before this trajectory, W34 is external-blocked. A mock, another model,
an earlier Grok transcript, or a direct internal Go call cannot substitute for
the real-client gate.

## 8. Network Failure Discipline

The Mac mini is reached through the managed non-interactive SSH route. Network
commands may use a bounded retry for transport failure. Retries must not change
operation IDs and must rely on Vermory idempotency for mutating operations.
An interrupted run retains its completed receipts and database state for
inspection before retry; it is not silently restarted from a clean database.

## 9. Evidence And Acceptance

The committed machine snapshot must contain only sanitized values and hashes:

- source and target host labels, namespaces, canonical roots, attachment and
  Git commit hashes;
- exact binary revision, checksum, signed-manifest identity, schema version,
  and restricted-role evidence;
- pre-rebind resolution and zero-side-effect counts;
- rebind, replay, bridge, binding, continuity, and tenant-isolation receipts;
- Grok version, MCP protocol/tool list, sanitized tool events, artifact hash,
  delivery, proposed writeback, and replay receipts;
- reversal receipts and exact post-reversal binding/resolution state;
- every hard-gate result and any retained failed attempt.

The slice passes only if all 32 hard gates in the runtime case pass. Client
text alone is not evidence: the Git state, artifact bytes, MCP event stream,
and PostgreSQL ledger must agree.

## 10. Non-Claims

W34 does not claim automatic repository-intent inference, seamless movement of
uncommitted files, PostgreSQL cross-host migration, high availability, every
coding client, model ranking, long-duration SSH reliability, or arbitrary
repository topologies. Cursor Agent remains a separate frozen qualification
and cannot inherit the Grok result.
