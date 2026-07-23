# Vermory Architecture

This document is the contributor map. The product contract remains the
[Product Constitution](docs/superpowers/specs/2026-07-11-vermory-product-constitution.md).
Implementation details may evolve, but changes must preserve the contracts
below unless the constitution is deliberately revised.

## System Boundary

Vermory provides governed memory and context continuity for AI clients. Its
native semantic authority is PostgreSQL. Models, embeddings, lexical indexes,
caches, packets, and optional memory backends are replaceable or rebuildable
participants, not alternate sources of truth.

The three continuity modes are:

- **Workspace-backed continuity:** a stable workspace anchor connects the same
  work across supported coding clients and isolates different workspaces.
- **Conversation-backed continuity:** a thread, channel, contact, or named
  matter carries an ongoing non-workspace task without pooling unrelated topics.
- **Global Defaults:** a deliberately thin, explicitly governed layer for
  durable cross-context preferences and settings.

Cross-boundary movement is explicit through promote, link, export, adopt,
rebind, split, or merge semantics. Similarity alone is never authority to join
continuities.

## Semantic Loop

```text
client event or trusted source
-> resolve continuity and authorization
-> preserve the source observation
-> form reviewable memory candidates
-> govern activation, correction, replacement, retention, and forgetting
-> maintain authoritative PostgreSQL state
-> rebuild lexical or vector projections
-> compose bounded task context
-> deliver to a client
-> record the outcome as a new observation or candidate
```

Retrieval is not allowed to bypass authorization, continuity, lifecycle,
privacy, or deletion checks. A successful model answer is not permission to
promote its output directly into durable memory.

## Package Map

| Path | Responsibility |
|---|---|
| `cmd/vermory` | Cobra CLI, runtime commands, packaging entry point |
| `internal/domain` | Core identifiers and domain contracts |
| `internal/resolver` | Trusted workspace attachment probing, workspace/conversation resolution, and Global Defaults resolution |
| `internal/governance` | Candidate and governed-memory lifecycle |
| `internal/bridge` | Explicit continuity bridge operations |
| `internal/store/postgres` | Authoritative PostgreSQL persistence and migrations |
| `internal/runtime` | Shared request, retrieval, and formation runtime |
| `internal/mcpserver` | MCP stdio surface for coding clients |
| `internal/webchat` | Conversation/Web Chat runtime |
| `internal/operatorcli` | Explicit review and administration commands |
| `internal/provider` | Replaceable model-provider adapters |
| `internal/memorybackend` | Native and optional projection adapters |
| `internal/packet` | Task-aware context delivery |
| `internal/reality` | Frozen cases, validation, reports, and attestations |
| `reality/cases` | Public frozen real or synthetic trajectories |
| `integrations/openclaw` | OpenClaw client integration |
| `integrations/hermes` | Hermes `MemoryProvider` integration |
| `docs/evidence` | Proven execution records and explicit claim boundaries |

## Authority And Projection Rules

- PostgreSQL contains continuity identity, governed memory state, source and
  revision history, deletion state, and rebuild inputs.
- Projection rows and queues may be dropped and rebuilt without semantic loss.
- Optional backends must not decide lifecycle, authority, or deletion.
- Provider outages may reduce automation or relevance quality; they must not
  lose authoritative observations or relax safety gates.
- Deleted or superseded content must not return as current through any
  projection, cache, artifact, adapter, or historical-default path.

## Change Boundaries

A permanent schema entity, lifecycle state, service, provider dependency, or
release metric requires:

1. a real failure or trajectory;
2. a frozen expected and forbidden behavior contract;
3. a falsifiable hypothesis or ADR when the decision is durable;
4. a migration or rollback path;
5. evidence that states exactly what was and was not qualified.

Start with the smallest end-to-end vertical slice that preserves these
contracts. Do not add empty package scaffolding in anticipation of unspecified
future architecture.

## Canonical References

- [Product Constitution](docs/superpowers/specs/2026-07-11-vermory-product-constitution.md)
- [Reality-First Platform Design](docs/superpowers/specs/2026-07-11-vermory-reality-first-memory-platform-design.md)
- [Reality Program](docs/superpowers/specs/2026-07-11-vermory-reality-program.md)
- [Hypothesis Register](docs/superpowers/specs/2026-07-11-vermory-hypothesis-register.md)
- [Capability And Evidence Matrix](docs/capability-evidence-matrix.md)
- [Legacy Provider Evaluation Matrix](docs/evaluation-matrix.md)
- [Repository Workflow](docs/collaboration/repository-workflow.md)
