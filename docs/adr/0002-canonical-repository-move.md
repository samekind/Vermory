# ADR 0002: Canonical Repository Moves To samekind

Status: accepted

Date: 2026-07-19

## Context

Vermory began in the personal public repository `jstar0/Vermory`. The project
now needs organization-owned collaboration, protected review by more than one
maintainer, and a stable home that is independent of one personal account.

The repository already contains signed delivery evidence whose certificate
identity, GitHub run URLs, source revisions, and pull-request references name
`jstar0/Vermory`. Those records describe events that actually occurred and must
not be rewritten as if they ran under another repository identity.

## Decision

The canonical repository is:

```text
https://github.com/samekind/Vermory
```

It is an independent organization repository, not a GitHub fork.

The initial organization mirror preserved the complete source repository Git
history and exact cloud refs:

```text
main:                    2fe75531c7d8e85b6d7fadf39e2b42dd70ebaac7
agent/grok-cli-runtime:  3eb0c2af900d71d96963c0f5264ce9ed8c502549
tags:                    none
```

The original Draft PR was recreated in the organization repository. Because
its accumulated 83,911-byte body exceeds GitHub's current pull-request creation
limit, the new PR uses a bounded summary and retains the complete original body
as ordered migration comments. The four original issue comments are also
retained with author, timestamp, and source URL.

Repository labels, topics, merge policy, required checks, protected review,
linear history, administrator enforcement, secret scanning, push protection,
Dependabot security updates, private vulnerability reporting, Issue forms,
CODEOWNERS, and contribution templates are carried into the organization
repository.

## Working Convention

- Local `origin` points to `samekind/Vermory` and is the only normal push target.
- Local `upstream` points to `jstar0/Vermory` for historical comparison only.
- New branches, pull requests, issues, protected checks, and releases are created
  in `samekind/Vermory`.
- The personal repository is not used as a parallel source of truth.
- Any future mirror or archival policy is explicit and must not create two
  independently writable canonical repositories.

## Evidence Preservation

Historical evidence remains immutable in meaning:

- old GitHub Actions links continue to name `jstar0/Vermory`;
- the W24 Sigstore certificate identity remains bound to the original workflow
  and pull-request ref;
- frozen I04 fixture hashes are not changed merely because the canonical
  repository moved;
- new organization runs record `samekind/Vermory` identities as separate
  evidence.

## Consequences

Contributors use the organization repository from this point forward. The
organization's protected branch requires non-author approval, so a second
write-capable collaborator must review the migrated Draft PR before merge.

The original repository remains useful for validating historical URLs and
signatures but receives no routine development pushes.
