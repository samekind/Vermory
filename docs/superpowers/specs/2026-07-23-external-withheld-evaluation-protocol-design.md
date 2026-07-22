# Vermory External Withheld Evaluation Protocol Design

Status: implementation target

Date: 2026-07-23

## 1. Purpose

Vermory already rejects a repository-readable case that claims to be sealed
and verifies a minimal Ed25519 result attestation. The missing boundary is a
portable submission protocol that an evaluator outside the implementation
session can actually execute without revealing its cases, expected answers,
provider credentials, or detailed failure report.

This protocol qualifies the handoff and verification mechanism. It does not
turn a local fixture, CI job, or self-signed test result into external evidence.
A sealed result exists only after an evaluator that the implementation process
cannot read runs a private suite and signs the returned attestation.

## 2. Actors And Trust Boundary

The protocol has three actors:

- The submitter publishes one immutable Vermory artifact and a submission
  manifest. The submitter knows the manifest nonce; it is an anti-replay value,
  not a secret.
- The evaluator owns the private suite, expected answers, provider route,
  ephemeral PostgreSQL instances, detailed reports, and Ed25519 private key.
- The verifier owns the evaluator public key and checks the public submission,
  signed attestation, and exact implementation binding.

The evaluator must execute Vermory as untrusted code. The accepted execution
boundary uses an ephemeral database, an evaluator-owned provider proxy,
outbound network denial except for that proxy, disabled telemetry, and an
artifact destination controlled by the evaluator. The submission contains no
callback, credential, provider URL, database URL, or private key.

## 3. Submission Manifest V1

`reality/schema/external-evaluation-submission-v1.schema.json` describes the
public manifest. Its canonical JSON digest is the submission identity used by
the result attestation.

The manifest binds:

- protocol version and unique submission ID;
- random nonce, creation time, and expiry time;
- requested suite profile without naming private cases;
- exact source revision;
- public immutable artifact URI, filename, size, and SHA-256;
- product interfaces the evaluator may drive;
- supported runtime platforms;
- the required evaluator-owned database, provider, network, telemetry, and
  result-output boundary.

Interface and platform names are versioned tokens rather than a closed product
taxonomy. A later suite can exercise another public Vermory surface without
changing the core trust protocol. Lists are sorted and unique so one semantic
submission has one canonical digest.

Only an HTTPS artifact URI without credentials, query parameters, or fragments
is accepted. The evaluator verifies the downloaded bytes before execution.

## 4. Attestation V2

The existing version-1 attestation remains verifiable for historical evidence.
Version 2 adds the fields needed for an external run to bind to a submission:

- evaluator key ID derived from the exact Ed25519 public key;
- external run ID;
- protocol version and requested suite profile;
- submission digest and implementation artifact digest;
- private suite version;
- evaluator-owned detailed-result digest;
- aggregate case counts;
- named hard-gate results and non-answer-leaking failure categories;
- run time and signature.

The public attestation does not contain case text, expected answers, retrieved
context, model prompts, model output, credentials, or the detailed report.
Those records stay under evaluator control and may be released only through a
separate review decision.

The verifier rejects:

- a valid signature made by a key whose key ID does not match;
- a result for another submission, artifact, protocol, or suite profile;
- a run outside the submission validity interval;
- inconsistent case or hard-gate counts;
- a claimed hard-gate pass with a failed or unexecuted hard gate;
- mutation, unknown fields, malformed digests, or unsupported versions.

No production CLI command signs an attestation. Unit tests may create temporary
keys solely to prove verifier behavior.

## 5. Evaluator Execution Contract

For a qualifying external run, the evaluator:

1. Receives the public submission and evaluator-pinned public-key identity.
2. Verifies the submission schema, canonical digest, expiry, and artifact hash.
3. Launches the artifact in a fresh sandbox with an ephemeral PostgreSQL
   database and no implementation-controlled telemetry or callback route.
4. Exposes only an evaluator-owned provider proxy when a case needs a model.
5. Drives the declared public interfaces, including MCP, authenticated Web Chat,
   and operator governance surfaces where required by the suite profile.
6. Uses private randomized tenant, continuity, repository, thread, and event
   identities so the submitted implementation cannot key on public fixtures.
7. Scores zero-tolerance isolation, attachment, deletion, stale-state, source
   authority, and governance gates deterministically; open task quality remains
   separate and cannot override a hard-gate failure.
8. Stores the detailed report outside implementation access and signs only the
   minimal version-2 attestation.

When a failed private case is released as a public regression, the evaluator
adds a replacement withheld case before the next claimed sealed run.

## 6. Qualification Levels

The repository uses three distinct statements:

- `protocol-qualified`: public positive and negative tests prove that submission
  creation, digest binding, strict verification, and result binding work.
- `external-run-recorded`: a named external evaluator returned a valid signed
  attestation for the exact submission, regardless of pass or fail.
- `sealed-qualified`: the external attestation passes all declared hard gates
  and the evaluator boundary is documented as inaccessible to implementation.

Protocol qualification alone never removes the open requirement for a genuine
external run.

## 7. Acceptance Checklist

- [x] A deterministic submission can be created from an exact artifact.
- [x] Submission parsing rejects unknown fields, secret-bearing URIs, invalid
  lifetimes, duplicate lists, malformed revisions, and artifact mutation.
- [x] Submission digests are stable across JSON formatting and change when any
  bound implementation or execution field changes.
- [x] Historical version-1 attestations still verify.
- [x] Version-2 attestations verify only with the exact evaluator key and exact
  submission.
- [x] Submission, implementation, suite profile, protocol, validity interval,
  counts, hard-gate consistency, result digest, and signature are all enforced.
- [x] Positive and negative protocol tests are public and reproducible.
- [x] The production CLI exposes create/verify commands for submissions and a
  verify-only command for external attestations; it exposes no signer.
- [x] Public documentation states the sandbox and evidence boundary without
  claiming that an external run already occurred.
- [ ] Repository policy, full Go tests, workflow lint, and release configuration
  checks pass before the change is published.
