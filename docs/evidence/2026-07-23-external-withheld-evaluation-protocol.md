# External Withheld Evaluation Protocol

Date: 2026-07-23

Reality case: `I10-external-withheld-evaluation-protocol`

Status: `protocol-qualified`

## Qualified Boundary

Vermory can bind one exact immutable artifact into a strict public submission,
reproduce the submission's canonical digest independent of JSON formatting,
verify the artifact filename, size, and SHA-256, and verify a version-2
Ed25519 result attestation only when it matches the exact evaluator key,
submission, implementation, protocol, suite profile, validity interval,
aggregate counts, hard-gate statuses, and detailed-result digest.

The production CLI exposes:

- `reality-submission-create`;
- `reality-submission-verify`;
- `reality-attestation-verify`.

It does not expose an external attestation signer.

## Public Negative Controls

The deterministic tests reject:

- unknown submission or attestation fields;
- credential-bearing and query-bearing artifact URIs;
- malformed revisions, nonces, digests, timestamps, and list ordering;
- duplicate interfaces, platforms, and failure categories;
- artifact filename, size, or content mutation;
- another submission, implementation, protocol, suite profile, run interval,
  or evaluator public key;
- inconsistent case counts and hard-gate counts;
- a hard-gate pass when a gate failed or did not run;
- payload and signature mutation;
- unsupported future attestation versions.

Historical version-1 attestations remain verifiable.

## Execution Boundary

Every accepted submission declares evaluator-owned ephemeral PostgreSQL,
evaluator-owned provider proxying, outbound network denial except to that
proxy, disabled implementation telemetry, and evaluator-controlled detailed
artifacts. The schema contains no credential, callback, private-case, expected
answer, or private-key field.

The public contract, CLI, schemas, and tests are described in the
[external evaluator handoff guide](../integrations/external-evaluator.md) and
[protocol design](../superpowers/specs/2026-07-23-external-withheld-evaluation-protocol-design.md).

## Claim Boundary

This evidence proves the public handoff and verifier protocol. No independent
evaluator has yet returned a private-suite attestation for this implementation.
The result is therefore not `external-run-recorded` and not
`sealed-qualified`. A repository-readable fixture, local unit-test key, or
self-controlled CI run cannot upgrade the evidence level.
