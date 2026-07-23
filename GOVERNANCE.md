# Vermory Governance

Vermory is maintained as an evidence-driven open-source project. Governance is
designed to protect the product contracts and the integrity of published
evidence without requiring every implementation detail to be permanent.

## Roles

### Contributor

Anyone who reports a reproducible failure, proposes a reality case, improves
documentation, or submits code through a pull request.

### Reviewer

A contributor trusted to review within a documented area. Reviewers may approve
changes but do not receive repository administration or release authority by
default.

### Maintainer

A person with write or maintain access who is responsible for merge decisions,
security response, release qualification, and protecting constitutional
boundaries. The current founding maintainer is [`@jstar0`](https://github.com/jstar0).

Additional maintainers are added through a pull request that updates this file
and `.github/CODEOWNERS`. The pull request should identify their sustained
contributions, review areas, and repository permission level.

## Decision Classes

| Change | Required process |
|---|---|
| Documentation, tests, local refactor | Normal pull request and protected checks |
| User-visible capability | Real failure or trajectory, acceptance criteria, tests, and scoped evidence |
| Schema, lifecycle, authority, deletion, or continuity boundary | Design document or ADR, migration/rollback plan, and maintainer approval |
| Product Constitution change | Explicit constitution diff, rationale, affected cases, and approval from two maintainers when two are available |
| Security-sensitive change | Private coordination when disclosure would create risk, plus security regression coverage |
| Release | Protected checks, signed artifact verification, explicit maintainer decision, and draft-first publication |

When only one maintainer exists, constitutional or release decisions are
recorded in the pull request and remain open to retrospective review after a
second maintainer joins. This is not permission to bypass protected checks.

## Merge Policy

- Changes reach `main` through pull requests.
- Protected status checks must pass on the latest head.
- At least one approving review is required. Until another write-capable
  collaborator is invited, protected pull requests intentionally remain
  unmergeable rather than falling back to self-approval.
- Stale approvals are dismissed after reviewable changes.
- The latest push must be approved by someone other than its author.
- Review conversations must be resolved before merge.
- Squash merge is the repository merge method; force pushes and branch deletion
  on `main` are disabled.
- Maintainers do not use administrative bypass for ordinary delivery.

CODEOWNERS identifies responsible reviewers. Requiring a CODEOWNER approval is
enabled only after at least two maintainers are listed, so the founding
maintainer cannot accidentally make every pull request unmergeable.

## Evidence Integrity

- Evidence says exactly which client, model, provider, dataset, hardware,
  revision, and negative controls were executed.
- Public, `withheld_local`, sealed, official-dataset, translated-proxy, and
  inspired-case labels remain distinct.
- Failures are retained and classified; they are not removed to improve a
  score or narrative.
- A compatibility pass is not a model ranking, and a component test is not a
  platform-completion claim.
- Credentials, private transcripts, OIDC tokens, private keys, and personal
  absolute paths are never public evidence.

## Security And Conduct

Security reports follow [SECURITY.md](SECURITY.md). Conduct expectations follow
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). A maintainer involved in a reported
conflict does not make the sole enforcement decision when another maintainer is
available.

## Inactivity And Removal

Maintainer access may be reduced after sustained inactivity, repeated violation
of security or evidence policy, or loss of account control. The change is
recorded through a governance pull request when public disclosure is safe.
Emergency access removal may occur first when repository or credential safety
requires it.
