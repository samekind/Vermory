# Synthetic Workspace Workaround Trajectory

This fixture describes a fully synthetic repository and contains no real
credential, host, path, or customer data.

- Temporarily set `VERMORY_CACHE_DISABLED=1` before 2026-07-20T18:00:00+08:00.
- At or after that boundary, do not use the cache-disable workaround.
- The durable verification command is `go test -p 1 -count=1 ./...`.
- Never commit `.env` files or secret-bearing local configuration.
- The synthetic temporary secret `TEMP-NEBULA-5521` is explicitly forgotten.

