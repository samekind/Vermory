# Repository Workflow

This is the operational collaboration path for Vermory contributors and
maintainers.

## 1. Classify The Change

Use the smallest applicable class:

| Class | Examples | Required design/evidence |
|---|---|---|
| Maintenance | typo, test cleanup, dependency update | focused tests and no claim expansion |
| Defect | wrong binding, stale recall, provider failure handling | reproduction, regression test, affected invariant |
| Capability | new formation, bridge, client, retrieval, or operator behavior | real trajectory, expected/forbidden behavior, scoped evidence |
| Constitutional | authority, continuity, deletion, governance contract | constitution or ADR change, migration/rollback, maintainer approval |
| Security | leakage, deletion residue, credential exposure | private report first when disclosure creates risk |
| Release | snapshot, signing, tag, package, deployment | protected checks, signed artifact verification, explicit release decision |

If a simpler fix satisfies the frozen contract, use it. Do not promote a local
implementation preference into architecture without evidence.

## 2. Create The Work Item

Use the repository issue forms for public defects, reality cases, and capability
proposals. Security-sensitive reports use GitHub private vulnerability
reporting.

The work item should state:

- the real user or operational failure;
- the continuity and authorization scope;
- expected current behavior;
- forbidden behavior;
- the simplest relevant baseline;
- completion evidence;
- migration or rollback concerns.

## 3. Freeze Before Claiming

For a new capability, add or revise the reality case before implementation
changes intended to pass it. Freeze authorized source fixtures, event sequence,
anchors, current facts, forbidden facts, downstream task, deterministic checks,
and fixture hashes.

Changing expected output after seeing the implementation requires a recorded
case revision and rationale. Preserve the earlier failure.

## 4. Implement A Vertical Slice

Prefer an end-to-end slice over empty layers:

```text
real input
-> continuity resolution
-> observation and formation
-> governance and PostgreSQL authority
-> retrieval and context delivery
-> real client consumption where applicable
-> write-back, correction, and deletion
-> evidence
```

Tests must include negative behavior, not only the successful path.

## 5. Open The Pull Request

The pull request template is the delivery contract. Include:

- outcome and scope;
- linked issue, case, hypothesis, design, or ADR;
- exact verification commands and results;
- security/privacy impact;
- migration and rollback path;
- evidence level and explicit non-claims;
- retained failures.

Draft pull requests are appropriate for early design and evidence review. Do
not mark a pull request ready while required evidence or migrations are still
unknown.

## 6. Review

Review in this order:

1. product boundary and authority;
2. isolation, deletion, and privacy;
3. behavior and failure handling;
4. migration and operational recovery;
5. test and evidence integrity;
6. maintainability and style.

Reviewers should identify the exact file, behavior, and consequence. Style-only
preferences do not override established repository patterns.

## 7. Merge And Release

`main` is protected. Required checks run against the latest pull-request head,
and squash merge preserves a focused history. A pull-request snapshot is an
expiring signed test artifact, not a release.

A release requires:

1. protected checks on the intended revision;
2. verification of the complete signed payload manifest;
3. explicit maintainer approval;
4. a `v*` tag that creates a draft GitHub Release;
5. review of release notes and attached artifacts before publication.

## 8. Add A Collaborator

The repository owner invites the GitHub account with the minimum required role.
Use `triage` for issue/evidence coordination, `write` for active implementation,
and `maintain` only for contributors responsible for protected delivery.

After sustained contribution, add the person to `GOVERNANCE.md` and
`.github/CODEOWNERS` through a reviewed pull request. Do not share personal
access tokens or deploy keys as a substitute for repository access.
