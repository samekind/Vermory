# Cursor Agent Real-Client Qualification Plan

> Canonical design: `docs/superpowers/specs/2026-07-19-cursor-agent-real-client-design.md`

## Checklist

- [x] Freeze and validate `W04-canonical-repository-cross-client`, including
  source revision, current marker, same-name distractor, and deterministic
  artifact assertions.
- [x] Add deterministic regression coverage for the frozen case and repository
  policy coverage for the Cursor client boundary.
- [x] Build a failure-preserving W25 runner that creates isolated attempt
  directories, project-local MCP configuration, normalized metadata, hashes,
  and exit status without recording credentials.
- [x] Stage the current Vermory binary and a dedicated W25 PostgreSQL database
  on the Mac mini without `sudo`.
- [x] Seed two exact workspace continuities, supersede the personal-repository
  fact, and preserve the active same-name distractor.
- [x] Verify Cursor discovers the remote stdio MCP server and only the expected
  non-governance tool arguments.
- [x] Execute a fresh real Cursor Agent task and preserve account, quota,
  provider, approval, MCP, artifact, and write-back failures without
  substitution.
- [ ] Verify the artifact, exact delivery, isolation, proposed-only write-back,
  active-retrieval exclusion, and idempotent replay in PostgreSQL.
- [x] Run privacy and checksum verification over the retained Mac mini evidence
  root.
- [x] Publish W25 evidence and evaluation-matrix claims only after all real
  client gates pass; otherwise publish the exact blocked status and non-claims.
- [ ] Run the complete repository pull-request gate, commit without amendment,
  push only to `samekind/Vermory`, and update Draft PR 1 with the scoped result.
