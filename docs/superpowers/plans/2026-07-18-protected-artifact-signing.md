# Protected Artifact Signing Implementation Plan

Design: [Protected Artifact Signing Design](../specs/2026-07-18-protected-artifact-signing-design.md)

## Task 1: Freeze Reality Case

- [x] Add and freeze `I04-protected-artifact-signing` before workflow implementation.
- [x] Validate all public reality cases and update authoritative case counts.

## Task 2: Complete Release Manifest

- [x] Add one portable deterministic manifest generator and verifier.
- [x] Require exactly four Go archives plus checksums, OpenClaw, Hermes, and Hermes sidecar.
- [x] Test deterministic output, missing payload rejection, and modified payload rejection.

## Task 3: Protected OIDC Signing

- [x] Keep the ordinary test job without `id-token: write`.
- [x] Add a trusted same-repo post-test signing job with minimal permissions.
- [x] Sign the complete manifest with pinned Cosign and upload the Sigstore bundle.
- [x] Verify exact workflow identity and issuer plus modified-manifest and wrong-identity rejection.

## Task 4: Manual And Tag Workflow Contract

- [x] Apply the same complete manifest and signature contract to manual snapshots.
- [x] Attach the manifest and bundle to draft tagged releases.
- [x] Keep publication, tags, and releases absent during W24 qualification.

## Task 5: Automated Qualification

- [x] Pass full PostgreSQL, race, Reality, vet, module drift, OpenClaw, Hermes, and packaging gates.
- [x] Pass actionlint and static permission/identity assertions for both workflows.
- [x] Preserve every failed signing, verification, workflow, and evidence attempt.

## Task 6: Mac Mini Protected Evidence

- [x] Stream the exact signed artifact through the Qingdao reverse-management tunnel without local persistence.
- [x] Verify transport digest, payload manifest, Sigstore identity/issuer, negative controls, packages, and privacy.
- [x] Save a complete evidence manifest under the isolated W24 Mac mini root.

## Task 7: Protected Delivery

- [x] Write public evidence and update only proven documentation claims.
- [x] Commit without amending earlier commits and push exact heads.
- [x] Require and verify protected `test` and `sign-snapshot` checks.
- [x] Update Draft PR 1 with exactly one W24 section.
- [x] Keep the overall Vermory platform goal active.
