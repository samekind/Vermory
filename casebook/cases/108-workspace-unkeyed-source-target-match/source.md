# Workspace Unkeyed Source Target Match

The `release-control-unkeyed` workspace already has three governed facts:

```text
release.signing.mode: Production releases use a macOS keychain certificate.
deploy.api.timeout: The deployment API timeout is 800 ms.
release.attestation.format: Production releases publish a signed SLSA provenance statement.
```

A trusted revision of `deploy/production.yaml` now states:

```text
Production releases now use GitHub Actions OIDC keyless signing.
```

The source connector knows the exact revision and fact text but does not know
Vermory's `memory_key`. Vermory may ask a provider to choose one key from the
three current facts or abstain. The provider must select
`release.signing.mode`; it must not invent a key, use another tenant's static
credential policy, or directly change active memory.

The match creates a proposed replacement. Before operator acceptance, normal AI
context still contains the keychain fact. After acceptance, the OIDC fact is
current and the keychain fact is stale.

The next coder must create `release-control-policy.md` containing OIDC keyless
signing, the 800 ms timeout, and the SLSA provenance statement. It must not
contain the old keychain instruction or another tenant's static credentials.
