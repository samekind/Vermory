# External Evaluator Handoff

Vermory's external-evaluation protocol lets an independent evaluator run a
private suite against one exact release artifact without exposing private
cases, expected answers, provider credentials, or detailed reports to the
implementation process.

This guide prepares and verifies the handoff. It does not create a private
suite and does not sign an external result.

## Evidence Levels

| Level | Meaning |
|---|---|
| `protocol-qualified` | Public positive and negative tests verify submission creation, exact artifact binding, strict parsing, and attestation binding. |
| `external-run-recorded` | A named evaluator outside implementation access returned a valid signed attestation for the exact submission. |
| `sealed-qualified` | The externally recorded run passed every declared hard gate. |

A local hidden directory, private branch readable by the implementation
session, or locally signed fixture cannot produce the last two levels.

## Create A Submission

Build or download the exact immutable artifact first. Keep large artifacts and
temporary files outside the repository worktree.

```bash
vermory reality-submission-create \
  --artifact /secure-staging/vermory_linux_amd64.tar.gz \
  --artifact-uri https://github.com/samekind/Vermory/releases/download/vX.Y.Z/vermory_linux_amd64.tar.gz \
  --source-revision <exact-lowercase-git-revision> \
  --submission-id vermory-vx.y.z-core-1 \
  --suite-profile core-continuity-v1 \
  --interface workspace_mcp_stdio_v1 \
  --interface authenticated_web_chat_http_v1 \
  --interface operator_cli_v1 \
  --platform linux_amd64 \
  --platform linux_arm64 \
  --valid-for 168h \
  --output /secure-staging/submission.json
```

The command reads the artifact, records its filename, size, and SHA-256, sorts
the interface and platform lists, generates a random nonce when one is not
provided, and writes a strict version-1 submission.

The manifest always declares:

- an evaluator-owned ephemeral PostgreSQL database;
- an evaluator-owned provider proxy;
- outbound network denial except to that proxy;
- disabled implementation telemetry;
- evaluator-controlled detailed result artifacts.

The submission format has no fields for credentials, database URLs, provider
URLs, callbacks, private cases, or expected answers.

## Verify Before Handoff

```bash
vermory reality-submission-verify \
  --input /secure-staging/submission.json \
  --artifact /secure-staging/vermory_linux_amd64.tar.gz
```

The output reports the canonical submission digest and implementation digest.
The evaluator should repeat the same verification after downloading the public
artifact.

## Evaluator Runtime Boundary

The evaluator treats the submitted artifact as untrusted code and runs it in a
fresh sandbox. The evaluator owns the database, provider proxy, randomized
tenant and continuity identities, private cases, expected outputs, detailed
logs, and result storage. Vermory receives no implementation-controlled
callback or telemetry route.

The private suite may drive any declared public interface. The core continuity
profile is expected to cover workspace MCP, authenticated conversation HTTP,
operator governance, cross-scope isolation, conservative attachment, stale
state, source authority, deletion residue, and bridge behavior. Exact private
cases and thresholds remain evaluator-owned.

## Verify An External Result

The evaluator returns a version-2 attestation and publishes its Ed25519 public
key through an out-of-band identity channel. Verification requires both the
public key and the exact submission:

```bash
vermory reality-attestation-verify \
  --input /review/evaluator-attestation.json \
  --public-key <base64-ed25519-public-key> \
  --submission /secure-staging/submission.json
```

Verification rejects another evaluator key, submission, implementation,
protocol, suite profile, or validity interval; inconsistent counts; a false
hard-gate pass; unknown fields; and payload or signature mutation.

The production CLI intentionally exposes no attestation signing command. The
evaluator private key never enters the Vermory repository, submitter machine,
release artifact, or public evidence bundle.

## Result Retention

The public attestation contains aggregate counts, named hard-gate statuses,
minimal failure categories, and the SHA-256 of the evaluator-owned detailed
report. It does not contain private case text, expected answers, retrieved
context, prompts, model output, or credentials.

If a private failure is released as a public regression, the evaluator should
add a replacement withheld case before the next sealed qualification.
