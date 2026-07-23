# Contributing to Vermory

Vermory accepts code, documentation, evaluation cases, and reproducible failure reports.

Read these documents before changing product behavior:

- [Architecture](ARCHITECTURE.md)
- [Development Guide](DEVELOPMENT.md)
- [Governance](GOVERNANCE.md)
- [Product Constitution](docs/superpowers/specs/2026-07-11-vermory-product-constitution.md)
- [Repository Workflow](docs/collaboration/repository-workflow.md)

## Choose The Right Channel

- Use a bug issue for public reproducible defects that contain no sensitive data.
- Use a capability proposal for a new product behavior or durable architecture change.
- Use a reality-case proposal when contributing an authorized trajectory or benchmark mapping.
- Use GitHub private vulnerability reporting for leakage, credential exposure, deletion residue, or another issue whose public reproduction would create risk.

Do not use an issue to publish private transcripts, provider keys, tenant data, database dumps, or personal absolute paths.

## Branch And Pull Request Flow

1. Branch from the current protected `main` unless continuing an accepted branch.
2. Keep the change focused on one failure, capability, or maintenance outcome.
3. Freeze a reality case before implementing a new capability intended to pass it.
4. Add positive and negative tests.
5. Run the local gates in [DEVELOPMENT.md](DEVELOPMENT.md).
6. Open a pull request using the repository template and keep it draft until its acceptance boundary is complete.
7. Preserve failed runs and address review comments without rewriting shared history.

Pull-request snapshots are signed test artifacts. They are not published releases.

## Before Opening a Pull Request

Run:

```bash
bash scripts/repository-policy.sh
go test -p 1 -count=1 ./...
go test -count=1 -race ./internal/reality
go vet ./...
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
git diff --check
```

Keep changes narrow and explain which real failure, frozen case, or falsifiable hypothesis the change addresses.

## Reality Evidence Rules

- Never commit credentials, private raw transcripts, personal absolute paths, or unrelated user content.
- Minimize and anonymize authorized source excerpts before committing them.
- A readable repository fixture may be `public` or `withheld_local`; it may not be described as sealed.
- Freeze expectations and forbidden behavior before implementing a mechanism intended to pass the case.
- Preserve failed cases and baseline outputs. Do not delete evidence merely to improve a score.
- Synthetic data is appropriate for security, privacy, mutation, and load testing, but it must be identified as synthetic.
- State the exact client, model, provider, version, revision, and evidence level when they affect a claim.
- Treat a substitute client or provider as separate evidence rather than silently replacing a failed target.
- Do not report a pull-request snapshot, sample dataset, compatibility probe, or mock as a release or complete platform qualification.

## Architecture Changes

Before adding a permanent entity, service, lifecycle state, provider dependency, or release metric:

1. Map it to an existing hypothesis in the hypothesis register, or add a new falsifiable hypothesis.
2. Name the real case or operational failure that requires it.
3. Define how a simpler baseline will be compared.
4. Define rollback or migration behavior.

## Review Priorities

Reviewers prioritize:

1. product boundary and semantic authority;
2. tenant and continuity isolation;
3. deletion, supersession, and privacy behavior;
4. migration and operational recovery;
5. test and evidence integrity;
6. maintainability and style.

Resolve review conversations before merge. Do not use administrative bypass for ordinary delivery.

## Commit Style

Use concise imperative commit subjects, for example:

```text
feat: add conversation continuity candidate formation
test: freeze workspace rebind trajectory
docs: record retrieval ablation result
```

Do not amend commits already shared for review unless the reviewer explicitly requests history repair. The repository uses squash merge, so intermediate review commits may remain honest and focused.
