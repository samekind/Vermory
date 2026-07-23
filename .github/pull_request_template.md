## Outcome

<!-- Describe the user-visible or operational result, not only the files changed. -->

## Why This Change

<!-- Link the issue, real failure, reality case, hypothesis, design, ADR, or security advisory. -->

## Scope

- Continuity mode or operational boundary:
- Authoritative state affected:
- Client/provider surface affected:
- Explicitly out of scope:

## Product Contract

- [ ] PostgreSQL remains the only native semantic authority.
- [ ] Models/providers cannot silently grant authority to their output.
- [ ] Tenant, continuity, lifecycle, privacy, and deletion gates remain enforced before delivery.
- [ ] Global Defaults remain thin and explicitly governed.
- [ ] Cross-continuity movement is explicit rather than similarity-driven.
- [ ] This change does not revive Gemini CLI as an active client target.
- [ ] Not applicable; this pull request does not change product behavior.

## Verification

<!-- List exact commands and results. Do not write only "tests pass". -->

```text
command -> result
```

## Evidence And Claim Boundary

- Evidence level: `none` / `public` / `withheld_local` / `sealed attestation`
- Real clients/models actually executed:
- Retained failures or negative controls:
- This pull request proves:
- This pull request does not prove:

## Security And Privacy

- [ ] No credential, OIDC token, private key, private transcript, database dump, full environment, or personal absolute path is included.
- [ ] Fixtures are synthetic or authorized and minimized.
- [ ] Deletion, leakage, source-injection, and wrong-binding impact has been considered.
- [ ] Security-sensitive details are handled privately when public disclosure would create risk.

## Migration And Rollback

<!-- Describe schema/config migration, compatibility, and rollback. Write "not applicable" with a reason when appropriate. -->

## Delivery Checklist

- [ ] The change is focused and unrelated cleanup is excluded.
- [ ] Frozen expectations were added or revised before capability implementation.
- [ ] Positive and negative tests cover the affected contract.
- [ ] `bash scripts/repository-policy.sh` passes.
- [ ] Affected local tests, full Go tests, race tests, and `go vet` pass as applicable.
- [ ] OpenClaw and Hermes checks pass when their surfaces are affected.
- [ ] Documentation, evidence, and failure records match the implementation.
- [ ] The latest pull-request head passes protected `test` and `sign-snapshot` checks.
