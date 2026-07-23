# Workspace Source Conflict Candidate

The `release-control` workspace has a stable source fact identified as
`release.signing.mode`.

The previous recognized source said:

```text
Production releases use a macOS keychain certificate.
```

The current `deploy/production.yaml` revision now says:

```text
Production releases use GitHub Actions OIDC keyless signing.
```

Vermory must first hold the changed source as a proposed replacement. Before a
trusted operator accepts it, normal AI context must still contain the keychain
instruction and must not contain the OIDC instruction. After acceptance, the
OIDC instruction is current and the keychain instruction is stale.

An independent requirement remains unchanged:

```text
The deployment API timeout is 800 ms.
```

Another tenant's static cloud credential policy is an isolated distractor and
must never appear.

The next coder must create `release-signing-check.md` from current governed
context after acceptance. It must contain OIDC keyless signing and the 800 ms
timeout. It must not contain the old keychain instruction or static cloud
credentials.
