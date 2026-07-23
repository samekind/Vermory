# Repository Instructions For AI Coding Agents

These rules apply to AI-assisted changes in this repository.

## Read Before Changing Product Behavior

1. `docs/superpowers/specs/2026-07-11-vermory-product-constitution.md`
2. `ARCHITECTURE.md`
3. `CONTRIBUTING.md`
4. `DEVELOPMENT.md`
5. The active design, reality case, and evidence document for the affected area

Do not reinterpret the product as a generic vector store, a project-only
memory tool, or a model-ranking harness. Preserve workspace-backed continuity,
conversation-backed continuity, thin Global Defaults, explicit bridges, and
PostgreSQL semantic authority.

## Implementation Rules

- Start from the real failure and acceptance boundary; do not create speculative
  package scaffolding.
- Keep changes narrow and compatible with existing migrations and evidence IDs.
- Models may propose but cannot govern or silently activate their own output.
- Similarity alone cannot bind workspaces, merge matters, or promote Global
  Defaults.
- Deleted and superseded facts must remain ineligible across every delivery and
  projection path.
- Preserve every observed failure. Never edit a report to turn a failed run into
  a pass.
- Do not claim a client, provider, benchmark, scale, release, or deployment was
  qualified unless the referenced evidence actually executed it.
- Treat `cursor-agent` as a real external-client target, not as an alias for
  editor implementation delegation. Client substitutions produce separate
  evidence.
- Gemini CLI is retired. Do not add it as an active client target.

## Privacy And Credentials

- Never commit provider keys, OIDC tokens, private keys, database credentials,
  private raw transcripts, full environment dumps, or personal absolute paths.
- Use synthetic or authorized minimized fixtures.
- Public evidence must identify anonymization and source authorization.
- Do not route public provider evidence through a private NewAPI gateway.

## Verification

Run the affected tests during development and the complete pull-request gate
from `DEVELOPMENT.md` before claiming completion. A mock or component test only
proves the boundary it exercised.

## Git Discipline

- Work through a focused branch and pull request.
- Do not amend or rewrite commits already shared for review.
- Do not revert unrelated contributor changes.
- Keep implementation, frozen cases, and evidence commits reviewable.
- Do not merge or publish a release without explicit maintainer authority.
