# External withheld evaluation boundary

This case contains only the public handoff and verification contract. It does
not contain a private case, expected answer, evaluator credential, provider
credential, database URL, signing private key, or detailed external report.

## Submission

The submitter binds one exact immutable Vermory artifact into a version-1
submission. The manifest names the protocol, submission ID, random nonce,
validity interval, suite profile, source revision, artifact URI, filename,
size, SHA-256, supported interfaces, runtime platforms, and evaluator execution
boundary.

The artifact URI uses HTTPS and contains no user information, query string, or
fragment. The evaluator verifies the downloaded artifact bytes before running
them. The submission digest is computed from validated canonical JSON so JSON
formatting does not create a different semantic submission.

The required execution boundary gives the evaluator an ephemeral PostgreSQL
database and evaluator-owned provider proxy. Network access is denied except to
that proxy, telemetry is disabled, and all detailed result artifacts stay under
evaluator control. The submission cannot name an implementation callback or
carry a credential.

## External result

A version-2 attestation binds the evaluator key ID, suite version, suite
profile, protocol version, submission digest, implementation digest, external
run ID, run time, aggregate counts, named hard-gate results, minimal failure
categories, and evaluator-owned detailed-result digest.

The verifier requires the exact evaluator Ed25519 public key and exact
submission. It rejects another submission or artifact, a run outside the
submission validity interval, inconsistent counts, a hard-gate pass when any
gate failed or did not run, unknown fields, payload mutation, and signature
mutation.

The public CLI can create and verify a submission and can verify an external
attestation. It cannot sign an external attestation. Temporary private keys in
unit tests exist only to prove verifier behavior and are not an evaluator
service or product signing surface.

## Evidence boundary

Passing the public protocol tests means `protocol-qualified`. It does not mean
an external evaluator ran a private suite. `external-run-recorded` requires an
attestation signed by a named evaluator outside implementation access.
`sealed-qualified` additionally requires that external result to pass all
declared hard gates.

A local directory, private branch readable by the implementation session,
self-hosted CI job controlled by the implementation, or locally signed fixture
cannot satisfy that boundary.
