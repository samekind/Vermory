# Linux Runtime Portability Evidence

Date: 2026-07-14

Revision under test: `429bc04fbb009d1b215fb163f12e564c1c1ff421`

## Scope

This evidence covers static Linux release builds for `amd64` and `arm64`, Linux process execution, unsafe server-configuration rejection, embedded migration availability outside the source tree, and database-backed authenticated-server startup.

The committed [release manifest](../../artifacts/release/2026-07-14/manifest.json) records checksums and deterministic probe results. The generated ELF binaries remain ignored build artifacts and are intended for a release packaging step rather than source control.

## Build

Both artifacts were built from the same clean revision:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath \
  -o artifacts/release/2026-07-14/vermory-linux-amd64 \
  ./cmd/vermory

CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go build -trimpath \
  -o artifacts/release/2026-07-14/vermory-linux-arm64 \
  ./cmd/vermory
```

| Artifact | Bytes | SHA-256 | Host metadata |
|---|---:|---|---|
| `vermory-linux-amd64` | `20,841,906` | `b94d5fecca146ff9dcad4353d6199491fdbaa6e8f680b55c4210ff91f3d074ba` | ELF 64-bit LSB, x86-64, statically linked |
| `vermory-linux-arm64` | `19,561,029` | `8b95e0d1b784168ca21586f296cf8151d52361654e4dc70c366f8eb0a004362f` | ELF 64-bit LSB, ARM aarch64, statically linked |

The same SHA-256 values were recomputed inside the Linux VM before execution.

## Release Migration Defect And Fix

The first `-trimpath` Linux database probe found a real packaging defect: `database migrate` looked for `internal/store/postgres/migrations` relative to the runtime working directory. A standalone release binary therefore failed outside the repository.

Revision `429bc04` fixes this by embedding the authoritative SQL migration directory into the binary with Go `embed.FS` and configuring goose to read that embedded filesystem. The regression test builds a `-trimpath` release binary, changes to an unrelated temporary directory, migrates a dedicated database, and asserts schema 9:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestOperationsRecovery/trimpath_release_binary_migrates_outside_repository' \
  -v
```

Result: `PASS`.

## Linux Runtime

The dedicated runtime was:

```text
Colima 0.10.3
macOS Virtualization.Framework
Linux 6.8.0-117-generic
guest architecture: aarch64
```

The arm64 binary ran directly in the same-architecture aarch64 VM. The amd64 binary ran through the VM's installed QEMU x86_64 binfmt handler. The amd64 result is emulated execution evidence, not native amd64 hardware evidence.

| Probe | arm64 | amd64 |
|---|---:|---:|
| `--help` exit | `0` | `0` |
| CLI usage present | yes | yes |
| non-loopback cleartext `serve` exit | `1` | `1` |
| rejected for missing TLS pair | yes | yes |
| database-backed missing-token HTTP | `401` | `401` |

The unsafe `serve` probe used a non-loopback listener without a TLS certificate/key pair. Both architecture builds rejected configuration before attempting to listen or connect to PostgreSQL.

## Database-Backed Execution

To avoid treating CLI startup as runtime proof, a dedicated PostgreSQL database and separate admin/runtime roles were created for the Linux probe. A temporary local TCP bridge exposed the host PostgreSQL 18.4 Unix socket only for the dedicated VM run.

The arm64 Linux binary executed:

```text
database migrate: exit 0, schema 9
database grant-runtime: exit 0
database rebuild-projections: exit 0, documents 0
```

The arm64 and emulated amd64 Linux binaries then each started `serve` with the restricted runtime identity. A request without a bearer token returned HTTP `401` from both processes, proving that each binary passed database-role validation, connected its authentication pool, bound its loopback listener, and enforced the authenticated boundary.

The PostgreSQL server itself ran on the host, not inside Linux. This evidence proves Linux Vermory client/runtime execution against PostgreSQL 18.4; it does not claim that PostgreSQL-on-Linux packaging or a native amd64 host was tested here.

## Cleanup

The dedicated database, admin role, runtime role, temporary TCP bridge, and `vermory-ops-i02-arm64` Colima profile were removed after capture. Existing `default`, `chatweb`, and running `contextmesh-eval` Colima profiles were not stopped or modified.

The final repository release gate reran the complete Go suite, race-sensitive runtime packages, `go vet`, module tidiness, OpenClaw checks, pack dry-run, Git whitespace checks, and host artifact-to-manifest SHA-256 comparison. All passed.
