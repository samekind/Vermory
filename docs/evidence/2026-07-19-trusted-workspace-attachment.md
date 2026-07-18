# Trusted Workspace Attachment Runtime Evidence

Date: 2026-07-19

Case: `W05-trusted-workspace-attachment`

Status: passed for the Codex real-client recall trajectory; failed attempts
remain part of the evidence record.

## Boundary Exercised

The client ran from a disposable synthetic Git checkout. A trusted local
launcher resolved the nested cwd to its canonical Git root, added the opaque
filesystem namespace `workstation-alpha`, calculated the bounded attachment
fingerprint, and passed the encoded attachment to Vermory MCP over SSH. The
MCP process and PostgreSQL 18 ran on the Mac mini. The server did not inspect
the workstation filesystem and the model did not receive a workspace path,
tenant, binding, namespace, adopt, or rebind input.

The authoritative database was a fresh schema 22 database. The operator first
confirmed the exact namespaced root and seeded two active facts:

```text
canonical_repository=Vermory
continuation_marker=vermory-w26-current
```

The seed used the operator CLI with `--filesystem-namespace`; it did not use a
model writeback or a provider gateway.

## Real Client Runs

| Client | Result | Evidence |
|---|---|---|
| Grok CLI 0.2.101 | blocked before generation | OIDC refresh returned `invalid_grant`; the client reported `Not signed in`, so it produced no MCP tool call. |
| Codex CLI 0.144.3, first natural-language replay | diagnostic failure | MCP resolved and writeback replay worked, but the model normalized the same natural-language repository fact differently in two artifacts. |
| Codex CLI 0.144.3, empty-context probe | failed recall gate | `prepare_context` correctly returned `resolved` but an intentionally broad task produced empty context; the client was required to stop, and this run was retained as a failure. |
| Codex CLI 0.144.3, governed recall replay | passed | The task named the governed fact types without supplying their values; MCP returned non-empty context, the client copied both lines byte-for-byte, and the writeback replay was idempotent. |

Codex used its normal authenticated client path. The MCP server was the real
stdio-over-SSH process, not an in-process fake or a scripted tool response.

## Passing Gates

The passing recall run proved all of the following from client events and the
Mac mini PostgreSQL ledger:

1. The trusted attachment resolved the exact namespaced binding.
2. `prepare_context` returned `status=resolved` and non-empty governed context
   for the semantic query `canonical repository` and `continuation marker`.
3. The client created `continuity-report-recall.md` with exactly two lines,
   copied from returned context in a different order but without changing
   either key or value.
4. The artifact SHA-256 was
   `0dc6a731a2c31d143cbdb3facb42b73db09e4bfd2245b072453ad65e2a2167fb`.
5. The first `commit_observation` returned `memory_status=proposed` and
   `replayed=false`.
6. The byte-identical second call returned the same observation and
   `replayed=true`.
7. PostgreSQL contained the exact namespaced binding, two prepared deliveries,
   one agent observation for the passing operation, and one proposed memory;
   no agent-result memory was active.
8. The MCP tool schema exposed only the normal prepare/commit workflow. A
   client-supplied `repo_root` is rejected as an unexpected additional
   property, and cannot override the startup attachment.

The retained raw client streams and sanitized SQL summaries are stored on the
Mac mini under the operator evidence root. Their artifact hashes include:

```text
codex initial stream: c672492e7de845529b12b0cbd92fa846a12ad1babcf6ffcd8fb936a900b6c193
codex empty-context stream: 1c893bd8a328b69de378d5a5d58306837ea75df867990b0c1622844252a7d419
codex passing recall stream: 1611559ff65653958b29aeb57451e31332f167105635a2f43a32a6670c0a71e2
passing artifact: 0dc6a731a2c31d143cbdb3facb42b73db09e4bfd2245b072453ad65e2a2167fb
```

## Scope And Non-Claims

This evidence qualifies one real Codex workspace trajectory and the shared
attachment contract. It does not qualify Grok generation, Cursor generation,
automatic worktree adoption, arbitrary clone identity, or model-independent
quality for every natural-language artifact format. The two failed Codex
trajectories remain visible because a resolved binding and a proposed
writeback alone are not proof that retrieval supplied the right governed facts.
