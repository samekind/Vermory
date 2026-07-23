# Release Control Policy

## Production signing mode
Production releases now use GitHub Actions OIDC keyless signing.

## Deployment API timeout
The deployment API timeout is 800 ms.

## Release attestation format
Production releases publish a signed SLSA provenance statement.
