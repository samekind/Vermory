# Token lifecycle fixture

All identities and credentials in this fixture are synthetic.

## Principals

- `identity-a / alice-client / client`
- `identity-a / alice-operator / operator`
- `identity-b / bob-operator / operator`
- one synthetic expired client principal used only for negative testing

## Required lifecycle

1. Issue tokens through the admin CLI and retain only public token IDs in evidence.
2. Store only SHA-256 digests in PostgreSQL.
3. Authenticate active tokens through the restricted security-definer lookup.
4. Reject malformed, unknown, expired, and revoked tokens without echoing token or tenant material.
5. Revoke the OpenClaw client token and apply revocation on the next request.
6. Preserve OpenClaw chat availability after revocation without claiming successful Vermory persistence.

No real token, digest, refresh credential, API key, or login artifact may enter the fixture, repository, logs, or evidence report.
