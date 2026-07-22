# Authenticated Remote Web Chat Qualification

Date: 2026-07-23

Runtime case: `W35-authenticated-remote-webchat`

Status: `26 / 26 PASS`

## Boundary Exercised

W35 qualifies Vermory's Web Chat as an authenticated remote browser surface,
not only a loopback application or an HTTP handler. The accepted run used
Google Chrome 150 over a public HTTPS origin. The browser reached an exact
OIDC-signed Darwin ARM64 release running as an unprivileged macOS LaunchAgent.
The Vermory process listened only on loopback and used PostgreSQL 18.3 at
schema 24 as the continuity, transcript, governance, token-lifecycle, and
projection authority.

The provider was the repository's deterministic provider with model label
`w35-deployment-contract`. That choice isolates remote delivery,
authentication, tenant isolation, authority, idempotency, restart, and failure
behavior. It is not a model-quality qualification. Existing Grok, DeepSeek,
OpenClaw, Hermes, and Codex evidence remains separate.

The complete machine-readable result is retained in the
[W35 runtime snapshot](snapshots/2026-07-23-authenticated-remote-webchat.json).

## Protected Release Provenance

The runtime source revision was
`7d0839d7f2935a933cab9477a82bdcae2d7252e8`. Protected pull-request run
[`29944895981`](https://github.com/samekind/Vermory/actions/runs/29944895981)
completed successfully. Its accepted jobs included:

| Job | Job ID | Result |
|---|---:|---|
| test | `89007674627` | success |
| Linux service lifecycle ARM64 | `89007674663` | success |
| Linux service lifecycle AMD64 | `89007674701` | success |
| native DEB/RPM package matrix | four native jobs | success |
| protected snapshot signing | `89008605335` | success |

The signed artifact was
`vermory-pr-snapshot-7d0839d7f2935a933cab9477a82bdcae2d7252e8`, artifact ID
`8539846545`, with artifact digest
`sha256:3aea9b703d0a233b5119bf699a77b6168227c0f5d7e6804cdc01cb2beb919443`.
The Darwin ARM64 archive digest was
`d78a70502df3922a892ae318e8a2c54e88cb4e204cecdf640d3bdbfa33de941f`;
the installed binary digest was
`83404b2a72316d6bebdf7410ba949b1fc82f3e8f1fdd35b82c5c63dbe003d31e`.
The installed binary reported revision `7d0839d7f2935a933cab9477a82bdcae2d7252e8`
and version `0.0.0-SNAPSHOT-7d0839d`.

Every payload matched `release-manifest.sha256` and `checksums.txt`. Cosign
verified the manifest against the protected workflow identity
`https://github.com/samekind/Vermory/.github/workflows/ci.yml@refs/pull/1/merge`.

## Accepted Remote Browser Trajectory

The deployed path was:

```text
Chrome
-> authenticated public HTTPS origin
-> reverse entrypoint and private relay
-> loopback-only Vermory service
-> PostgreSQL authority
```

The final clean Chrome load made 20 same-origin HTTPS requests. Every request
completed with HTTP `200`; there was no mixed content, console error, warning,
cross-origin credential request, or horizontal overflow. The page was a secure
context and the deployed certificate was valid through 2026-10-20.

The unauthenticated application shell and same-origin assets loaded without an
API credential. The public runtime probe disclosed only the minimum deployment
mode needed to present the login surface. Protected session access returned
`401` without a credential.

After login, the raw credential existed only in current page memory. Shape
checks found no credential in the URL, rendered DOM, local storage, or session
storage. Refresh required reauthentication, then recovered the selected thread
and authoritative server transcript from PostgreSQL. Browser storage retained
only tenant-scoped thread labels, selection, and pending operation metadata.

## Tenant And Authority Gates

Two tenants used the exact same `web_chat` thread anchor. PostgreSQL recorded
two bindings, two distinct tenants, and two distinct continuity IDs. Each
tenant saw its own marker and did not receive the other tenant's marker.

The client role could send and inspect its own conversation. Its browser UI
showed no governance controls and did not render the other tenant's transcript
or marker. Direct attempts to list memory candidates or mutate a Global
Default returned `403`. The operator role could confirm, correct, and forget
memory within its own tenant.

A revoked token returned `401`; another active token still returned `200`.
Revocation did not delete tenant memory or disclose the revoked credential.

## Governed Memory Lifecycle

The first exact lifecycle used marker `EXACT-35-ECHO`:

```text
operator login
-> send source turn
-> Remember
-> fresh turn receives the marker through Governed memory
-> Forget
-> browser current-memory count becomes zero
-> fresh turn no longer receives the marker
```

The final database assertion for that memory was:

```text
deleted | content redacted | origin observation retained
lexical projection rows: 0
vector projection rows: 0
2560-dimensional vector projection rows: 0
```

For correction, the browser formed `EXACT-CORRECT-ALPHA`, corrected the current
memory to `EXACT-CORRECT-BETA`, and linked an independent target conversation
through the explicit bridge API. The target conversation's fresh assistant
turn contained `Governed memory:` and `EXACT-CORRECT-BETA`, did not contain
`EXACT-CORRECT-ALPHA`, and did not pool the source transcript. The corrected
memory was then forgotten; the source browser memory view returned to zero and
the database again reported `deleted|true|true|0|0|0` for lifecycle, content
redaction, retained origin, lexical rows, vector rows, and 2560-dimensional
vector rows.

Forgetting governed memory did not erase the source transcript. This is the
frozen W35 contract: current recall and retrieval projections forget the
memory, while source evidence remains available for audit according to its own
retention policy.

## Revocation, Retry, Restart, And Relay Failure

The browser persisted a pending operation ID before issuing its request, but
never persisted the credential. When the active token was revoked, the next
send returned the UI to login, kept the operation in a recoverable failed
state, and committed zero database turns. After replacement authentication,
Retry reused the original operation ID. PostgreSQL contained exactly one turn
and one distinct operation ID; the pending state cleared.

The relay-loss trajectory used the same idempotency rule. With the public relay
disabled, the request returned `502` and PostgreSQL contained zero rows for the
operation. After the relay was restored, retrying the same operation ID
returned `200` and committed exactly one row. No phantom assistant result was
created during failure.

After an exact service restart, the public root returned `200`, authenticated
session access returned `200`, eight observations remained, and the forgotten
memory remained deleted. The service continued as an unprivileged user-level
LaunchAgent; no root-scoped runtime deployment was used.

## Credential And Transport Checks

Final scans found no raw token in:

- macOS service logs;
- the installed service directory excluding its private environment file;
- PostgreSQL content;
- reverse-entrypoint access or error logs;
- browser URL, DOM, local storage, session storage, or model context.

The same-origin application retained restrictive Content Security Policy,
`no-referrer`, `nosniff`, and frame-ancestor protections over HTTPS.

## Claim Boundary

W35 qualifies the exact signed remote Chrome deployment for authenticated
login, role-gated browser behavior, tenant-scoped continuity, governed memory
confirmation/correction/forgetting, token revocation, refresh recovery,
idempotent retry, process restart, relay failure, credential non-persistence,
and PostgreSQL-backed deletion projections.

It does not claim:

- real-model quality from the deterministic provider;
- browser support beyond the accepted Chrome version;
- Internet-scale availability, latency, or service-level objectives;
- a general-purpose identity product;
- that the browser, relay, or transcript is the memory authority;
- that forgetting governed memory erases the source transcript.

