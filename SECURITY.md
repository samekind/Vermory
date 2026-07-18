# Security Policy

## Supported Code

Security fixes target the protected `main` branch. Pre-release snapshots and
older commits receive best-effort analysis but are not maintained release
lines. When versioned releases exist, this section will list their support
window explicitly.

## Reporting a Vulnerability

Do not open a public issue for a vulnerability that could expose memory content, credentials, tenant data, deleted facts, or continuity bindings.

Use GitHub's private vulnerability reporting or Security Advisory flow for `samekind/Vermory`. Include:

- affected commit or version;
- reproduction steps with synthetic data;
- expected and observed isolation or deletion behavior;
- whether optional adapters, caches, artifacts, or logs retain the target;
- any known mitigation.

Never include real credentials, private transcripts, or personal memory exports in the report.

Maintainers will acknowledge a private report as soon as practical, reproduce
it with synthetic data where possible, and coordinate disclosure after a fix or
mitigation is available. Do not demand that a reporter publish sensitive proof.

## High-Priority Security Boundaries

- cross-tenant and cross-continuity isolation;
- wrong workspace binding;
- deleted-fact residue;
- source prompt injection and unauthorized promotion;
- optional adapter deletion propagation;
- artifact, cache, and audit redaction;
- attestation signature verification.

## Repository Security Controls

- secret scanning and push protection are enabled;
- private vulnerability reporting is enabled;
- protected pull requests require CI and signed-snapshot checks;
- release snapshots use GitHub OIDC keyless signing rather than a long-lived
  repository signing key;
- provider and deployment credentials are never accepted as public fixtures.
