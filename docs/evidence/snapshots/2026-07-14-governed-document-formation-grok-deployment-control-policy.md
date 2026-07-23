# Deployment Control Policy

## Primary production region
Production deploys to us-east-1.

## Deployment retry limit
Production deployments retry at most 5 times.

## Rollback approval
Rollback approval requires two maintainers.

## Release attestation
Production releases publish a signed SLSA provenance statement.
