# External Withheld Evaluation Protocol

Date: 2026-07-23

Reality case: `I10-external-withheld-evaluation-protocol`

Status: `protocol-qualified`

Accepted implementation head:
`bb3c919efbd298f47f61fb781d6e6924480a44a4`

Machine-readable snapshot:
[2026-07-23-external-withheld-evaluation-protocol.json](snapshots/2026-07-23-external-withheld-evaluation-protocol.json)

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

## Protected CI And Artifact Binding

The exact implementation head passed
[GitHub Actions run 29962727959](https://github.com/samekind/Vermory/actions/runs/29962727959).
The accepted jobs include:

- main `test`: `89067294746`;
- protected `sign-snapshot`: `89068059759`;
- two Linux service lifecycle jobs;
- four native package jobs;
- four signed APT/DNF repository jobs;
- four cross-version repository lifecycle jobs;
- two versioned-package prerequisite jobs.

All jobs completed successfully. The main test job passed repository policy,
PostgreSQL tests, runtime and reality race tests, vet, module cleanliness,
release build, OpenClaw packaging, Hermes packaging, complete release manifest,
and clean-diff checks.

The signed artifact is
`vermory-pr-snapshot-bb3c919efbd298f47f61fb781d6e6924480a44a4`, artifact
ID `8546725792`. Its GitHub digest and independently downloaded ZIP SHA-256
both equal:

```text
7c42b067ec2776d0a45f31d9ae11d947634464ceffb35f10b519fdc672ed7b5f
```

The 16-entry release manifest SHA-256 is:

```text
d4831976f1a5bb8eea80b5b838c4a4d85433fe6d364ca10c882ef8f96570f8cb
```

The Sigstore bundle SHA-256 is:

```text
24e5671d4a70896fe623cc008245402457c1cb2075cd5adb58765aa4bf5da737
```

Independent Cosign verification passed only for the exact workflow identity
`https://github.com/samekind/Vermory/.github/workflows/ci.yml@refs/pull/1/merge`
and issuer `https://token.actions.githubusercontent.com`. A modified manifest
and a wrong workflow identity were both rejected. The downloaded Darwin ARM64
binary reported revision
`bb3c919efbd298f47f61fb781d6e6924480a44a4`.

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
