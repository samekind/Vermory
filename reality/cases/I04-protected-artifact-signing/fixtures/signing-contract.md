# Protected artifact signing qualification fixture

All repository names, workflow identities, revisions, artifact names, and
payload filenames in this case refer to Vermory's public protected-delivery
contract. No private signing key, provider credential, database secret, or
user data is part of the case.

## Signed subject

The signed subject is one deterministic SHA-256 manifest named
`release-manifest.sha256`. It covers exactly these release payload classes:

1. four Go archives: Darwin amd64, Darwin arm64, Linux amd64, Linux arm64;
2. the GoReleaser `checksums.txt` file;
3. the OpenClaw package;
4. the Hermes package;
5. the Hermes package SHA-256 sidecar.

The Sigstore bundle is not recursively listed in the manifest it authenticates.
The GitHub artifact transport ZIP is also not the signed subject because GitHub
creates that container after the workflow uploads the payload.

## Signing authority

Protected snapshots use keyless Sigstore signing with a short-lived certificate
issued from GitHub's OIDC identity. No long-lived signing private key is stored
in the repository, GitHub secrets, Mac mini, workstation, or release bundle.

The certificate identity must equal the exact workflow path and Git reference
that created the snapshot. Verification also requires GitHub's OIDC issuer.
For a pull-request snapshot, GitHub API evidence must independently prove that
the signed synthetic merge has the expected source head as its second parent.

## Workflow isolation

The ordinary test job does not receive `id-token: write`. A separate signing
job runs only after all protected tests pass and only for a trusted same-repo
pull request or a push to the repository's protected branch. An untrusted fork
pull request may run tests but cannot obtain the Vermory signing identity.

## Required verification

Acceptance requires all of the following:

- the complete manifest contains exactly eight sorted payload records;
- every listed payload hash verifies;
- the Sigstore bundle verifies the exact manifest bytes;
- certificate identity and OIDC issuer match the expected GitHub workflow;
- a modified manifest fails verification;
- the correct manifest fails under a wrong workflow identity;
- absence of the bundle is not treated as signed delivery;
- Mac mini verification matches the GitHub artifact metadata and source head;
- no credential, private key, OIDC token, or full environment enters evidence.

## Claim boundary

This qualification proves a signed protected snapshot and configures the same
contract for manual/tag workflows. It does not publish a GitHub Release, create
a tag, notarize macOS binaries, sign a container image, prove external sealed
evaluation, or declare final release acceptance.
