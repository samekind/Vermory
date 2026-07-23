# Identity, Authorization, And PostgreSQL RLS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a self-hostable authenticated multi-tenant HTTP profile whose tenant and role come from server-issued tokens and whose PostgreSQL runtime role remains isolated by RLS even when application tenant filters are omitted.

**Architecture:** Keep the existing local `web-chat` and MCP commands unchanged. Add a restricted token schema plus security-definer lookup, tenant-aware foreign keys and RLS policies, an RLS-aware runtime store pool, an authenticated handler that creates tenant-scoped services per request, and a separate `serve` command that refuses unsafe database roles and cleartext non-loopback listeners. OpenClaw reads an optional bearer token only from `VERMORY_API_TOKEN`.

**Tech Stack:** Go, PostgreSQL 17 test runtime with PostgreSQL 16+ compatible SQL, pgx v5, goose, Cobra, `net/http`, TypeScript 5, Vitest, pnpm 11, OpenClaw 2026.6.11, local authenticated Grok CLI.

## Global Constraints

- Existing local `web-chat`, `mcp-stdio`, Operator CLI, and unauthenticated loopback behavior remain compatible.
- Authenticated `serve` never accepts `tenant_id`, role, continuity IDs, lifecycle state, or provider authority from request JSON.
- Tokens use `vmt_<public-id>_<secret>` and PostgreSQL stores only a SHA-256 digest.
- Roles are fixed to `client`, `operator`, and `owner`.
- The serving database role must not be superuser, table owner, or `BYPASSRLS`.
- `serve` never runs migrations and never falls back to an admin database connection.
- Non-loopback `serve` requires both TLS certificate and key.
- RLS protects only the tenant-bearing tables granted to the runtime role; legacy project/source/capsule tables receive no runtime-role grants.
- The OpenClaw token comes only from `VERMORY_API_TOKEN`; it is never accepted as plugin config or included in logs/errors/model context.
- Database-mutating Go tests run serially with `VERMORY_TEST_DATABASE_URL`.
- No Gemini CLI and no Mac mini NewAPI routing.

---

### Task 1: Freeze I01-I06 Authenticated Multi-Tenant Case

**Files:**
- Create: `reality/cases/I01-authenticated-multitenant-rls/manifest.json`
- Create: `reality/cases/I01-authenticated-multitenant-rls/events.jsonl`
- Create: `reality/cases/I01-authenticated-multitenant-rls/fixtures/token-lifecycle.md`
- Create: `reality/cases/I01-authenticated-multitenant-rls/fixtures/tenant-isolation.md`
- Create: `reality/cases/I01-authenticated-multitenant-rls/fixture-lock.json`
- Modify: `internal/reality/validate_test.go`
- Modify: `internal/reality/experiment0.go`
- Modify: `internal/reality/experiment0_test.go`

**Interfaces:**
- Consumes: existing frozen reality case format and fixture-lock validation.
- Produces: immutable identities, role actions, forbidden outcomes, RLS attacks, pool-reuse order, and authenticated OpenClaw replay checks used by later acceptance.

- [x] **Step 1: Write the I01 trajectory**

Use tenants `identity-a` and `identity-b`, subjects `alice-client`, `alice-operator`, and `bob-operator`, the same conversation anchor under both tenants, one A-only memory, one revoked OpenClaw client token, and explicit filter-omission/cross-tenant-FK attack events.

- [x] **Step 2: Add failing reality validation**

Assert pressures include `token_expiry`, `token_revocation`, `role_denial`, `same_anchor_cross_tenant`, `filter_omission`, `cross_tenant_foreign_key`, `pool_reuse`, and `authenticated_openclaw`.

- [x] **Step 3: Verify RED**

```bash
go test -count=1 ./internal/reality -run 'Test.*I01'
```

Expected: FAIL because I01 and its lock are absent.

- [x] **Step 4: Freeze exact fixture hashes**

Use SHA-256 plus byte counts in the existing format. Keep token values synthetic labels only; never freeze a real credential.

- [x] **Step 5: Verify and commit**

```bash
go test -count=1 ./internal/reality
git diff --check
git add reality/cases/I01-authenticated-multitenant-rls internal/reality
git commit -m "test: freeze authenticated multi-tenant case"
```

### Task 2: Auth Schema, Tenant-Aware Foreign Keys, And RLS Migration

**Files:**
- Create: `internal/store/postgres/migrations/00009_identity_authorization_rls.sql`
- Modify: `internal/runtime/postgres_store.go`
- Create: `internal/runtime/rls_migration_test.go`

**Interfaces:**
- Consumes: migrations 00001-00008 and all tenant-bearing served tables.
- Produces:

```sql
vermory_auth.api_tokens
vermory_auth.authenticate_token(public_id text, digest bytea)
```

plus RLS policies and tenant-aware foreign keys for the 12 served tables.

- [x] **Step 1: Write failing migration tests**

After migration, assert:

- restricted `vermory_auth` schema and token table exist;
- raw token secret columns do not exist;
- token role/status checks exist;
- security-definer lookup has a fixed search path and is not executable by `PUBLIC`;
- all 12 served tables have RLS enabled;
- every served relationship has a tenant-aware composite foreign key;
- default/`PUBLIC` privileges expose neither auth rows nor legacy `projects`, `sources`, `claims`, `capsules`, `packets`, or `wcef_runs` tables.

- [x] **Step 2: Verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime -run 'TestIdentityRLSMigration' -v
```

Expected: FAIL because migration 00009 is absent.

- [x] **Step 3: Implement token schema and lookup function**

Include `issue_operation_id`, request fingerprint, public token ID, digest, tenant, subject, role, status, expiry, created/revoked timestamps, and idempotency uniqueness. Revoke all schema/table/function privileges from `PUBLIC`.

- [x] **Step 4: Add tenant-aware keys and RLS**

Add composite unique constraints and foreign keys, validate existing rows, enable RLS, and add `USING`/`WITH CHECK` policies comparing `tenant_id` with `nullif(current_setting('vermory.tenant_id', true), '')`.

- [x] **Step 5: Extend test reset**

`ResetForTest` truncates auth tokens only when running through an admin-capable local test store. It must not grant runtime access to auth rows.

- [x] **Step 6: Verify migration and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime -run 'TestIdentityRLSMigration' -v
git diff --check
git add internal/store/postgres/migrations/00009_identity_authorization_rls.sql internal/runtime/postgres_store.go internal/runtime/rls_migration_test.go
git commit -m "feat: add identity schema and tenant RLS"
```

### Task 3: Token Authentication, Runtime Role Provisioning, And Identity CLI

**Files:**
- Create: `internal/authn/types.go`
- Create: `internal/authn/token.go`
- Create: `internal/authn/postgres.go`
- Create: `internal/authn/provision.go`
- Create: `internal/authn/token_test.go`
- Create: `internal/authn/postgres_test.go`
- Create: `internal/identitycli/command.go`
- Create: `internal/identitycli/command_test.go`
- Modify: `cmd/vermory/main.go`
- Modify: `cmd/vermory/main_test.go`

**Interfaces:**
- Produces:

```go
type Role string
const (
    RoleClient Role = "client"
    RoleOperator Role = "operator"
    RoleOwner Role = "owner"
)

type Principal struct {
    TokenID   string
    TenantID  string
    SubjectID string
    Role      Role
    ExpiresAt time.Time
}

type Authenticator interface {
    Authenticate(context.Context, string) (Principal, error)
}

func IssueToken(context.Context, *pgxpool.Pool, IssueTokenRequest) (IssueTokenReceipt, error)
func RevokeToken(context.Context, *pgxpool.Pool, RevokeTokenRequest) (TokenInspection, error)
func InspectToken(context.Context, *pgxpool.Pool, string) (TokenInspection, error)
func GrantRuntimeRole(context.Context, *pgxpool.Pool, string) error
```

- [x] **Step 1: Write token unit tests**

Cover format, cryptographic entropy source injection, bounded parsing, digest determinism, invalid role/tenant/subject/expiry, and no secret in `String`, JSON inspection, or errors.

- [x] **Step 2: Verify RED**

```bash
go test -count=1 ./internal/authn -run 'TestToken' -v
```

- [x] **Step 3: Implement minimal token primitives**

Use `crypto/rand`, base64url without padding, SHA-256, constant bounded lengths, and typed safe errors.

- [x] **Step 4: Write PostgreSQL lifecycle tests**

Cover issue replay, conflicting replay, authenticate, expiry, revoke, cross-tenant metadata isolation, and a runtime role that can execute the lookup function but cannot select `vermory_auth.api_tokens` or any legacy project/source/capsule table.

- [x] **Step 5: Implement lifecycle and role grants**

Use admin transactions for issue/revoke. `GrantRuntimeRole` validates the role exists and is neither superuser nor `BYPASSRLS`, then grants only the served-table operations, required sequences, schema usage, and token lookup execution.

- [x] **Step 6: Write CLI tests**

Cover:

```text
identity token issue
identity token inspect
identity token revoke
database grant-runtime
database migrate
```

Require explicit admin database URL. Token issue prints the secret once; inspect/revoke never print it.

- [x] **Step 7: Implement commands and register them**

Keep token lifecycle out of HTTP. `database migrate` uses the existing runtime migration source through an admin store; `database grant-runtime` accepts a validated PostgreSQL role identifier.

- [x] **Step 8: Verify and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/authn ./internal/identitycli ./cmd/vermory
git diff --check
git add internal/authn internal/identitycli cmd/vermory
git commit -m "feat: add token identity management"
```

### Task 4: RLS-Aware Runtime Store Pool

**Files:**
- Create: `internal/runtime/tenant_context.go`
- Modify: `internal/runtime/postgres_store.go`
- Modify: `internal/runtime/conversation_store.go`
- Modify: `internal/runtime/global_defaults_store.go`
- Modify: `internal/runtime/bridge_store.go`
- Create: `internal/runtime/tenant_pool_test.go`

**Interfaces:**
- Produces:

```go
type StoreOptions struct {
    EnforceTenantContext bool
}

func OpenStoreWithOptions(context.Context, string, StoreOptions) (*Store, error)
func (s *Store) ValidateRuntimeRole(context.Context) error
```

- [x] **Step 1: Write failing pool/RLS tests**

Create a non-owner test role, grant runtime privileges, then prove:

- tenant A and B direct queries without tenant predicates see only their own rows;
- missing tenant context sees no rows or fails closed;
- a B row cannot reference an A continuity/memory/delivery/bridge UUID;
- A/B/A/B pool reuse and concurrency never carry stale tenant settings;
- `ValidateRuntimeRole` rejects superuser, `BYPASSRLS`, and table owner identities.

- [x] **Step 2: Verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime -run 'TestTenantPool|TestRuntimeRole' -v
```

- [x] **Step 3: Implement tenant context and pool hooks**

`BeforeAcquire` sets the server-derived tenant GUC. `AfterRelease` resets it with a bounded context and discards the connection if reset fails. Missing tenant context fails closed.

- [x] **Step 4: Scope every tenant-bearing store entry point**

At the beginning of every public store method that accepts `tenantID`, attach the normalized tenant to the context before any pool operation. Private helpers preserve that context. Migration/reset methods remain admin-only and do not use the enforced pool.

- [x] **Step 5: Implement runtime-role validation**

Check `current_user`, `rolsuper`, `rolbypassrls`, and ownership of served tables. Return a safe startup error without connection strings or role passwords.

- [x] **Step 6: Verify all runtime tests and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime
git diff --check
git add internal/runtime
git commit -m "feat: enforce tenant context in runtime store"
```

### Task 5: Authenticated HTTP Handler And `serve` Command

**Files:**
- Create: `internal/webchat/authenticated_handler.go`
- Create: `internal/webchat/authenticated_handler_test.go`
- Create: `cmd/vermory/serve.go`
- Create: `cmd/vermory/serve_test.go`
- Modify: `cmd/vermory/main.go`

**Interfaces:**
- Consumes: `authn.Authenticator`, RLS-aware `runtime.Store`, existing provider construction, and existing tenant-scoped handlers.
- Produces:

```go
func NewAuthenticatedHandler(
    store *runtime.Store,
    provider provider.Provider,
    model string,
    authenticator authn.Authenticator,
) http.Handler
```

and `vermory serve`.

- [x] **Step 1: Write failing authentication/authorization tests**

Cover missing/malformed/unknown/expired/revoked token `401`, client-role governance `403`, operator success, safe cross-tenant not-found behavior, no accepted tenant field, and no token/tenant/resource leakage in errors.

- [x] **Step 2: Verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/webchat -run 'TestAuthenticated' -v
```

- [x] **Step 3: Implement principal middleware and route policy**

Parse one Bearer credential, authenticate it, authorize the route, then construct existing conversation/default/bridge services with `principal.TenantID`. Do not cache tenant-bound services across principals.

- [x] **Step 4: Write failing `serve` tests**

Require runtime DB URL and listen address, reject implicit migrations, reject unsafe runtime role, reject non-loopback without both TLS files, and accept loopback without TLS.

- [x] **Step 5: Implement `serve`**

Open one plain auth pool and one RLS-enforced store pool from the runtime DSN. Validate the runtime role. Start `http.Server` with the existing timeouts; use `ListenAndServeTLS` only when certificate/key are present. Keep `web-chat` unchanged.

- [x] **Step 6: Verify and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/webchat ./cmd/vermory
git diff --check
git add internal/webchat cmd/vermory
git commit -m "feat: add authenticated multi-tenant server"
```

### Task 6: OpenClaw Bearer Token Environment Boundary

**Files:**
- Modify: `integrations/openclaw/src/client.ts`
- Modify: `integrations/openclaw/src/index.ts`
- Modify: `integrations/openclaw/test/client.test.ts`
- Modify: `integrations/openclaw/test/plugin.test.ts`
- Modify: `docs/integrations/openclaw-runtime.md`

**Interfaces:**
- Consumes: environment variable `VERMORY_API_TOKEN`.
- Produces: optional `Authorization: Bearer ...` on Vermory requests without adding token fields to plugin configuration.

- [x] **Step 1: Write failing client/plugin tests**

Assert token env is trimmed, malformed whitespace/control characters are rejected, valid token produces the exact header, local no-token requests omit the header, and errors/logs never contain the token.

- [x] **Step 2: Verify RED**

```bash
PATH=/opt/homebrew/opt/node@24/bin:/opt/homebrew/bin:/usr/bin:/bin \
  pnpm -C integrations/openclaw test -- client.test.ts plugin.test.ts
```

- [x] **Step 3: Implement minimal bearer support**

Read the environment once during plugin registration, pass an optional token to the client, and set only the Authorization header. Do not add config schema fields or model-facing text.

- [x] **Step 4: Update runbook and verify**

```bash
PATH=/opt/homebrew/opt/node@24/bin:/opt/homebrew/bin:/usr/bin:/bin \
  pnpm -C integrations/openclaw check
git diff --check
git add integrations/openclaw docs/integrations/openclaw-runtime.md
git commit -m "feat: authenticate OpenClaw requests"
```

### Task 7: I01-I06 Deterministic Acceptance And Real OpenClaw Replay

**Files:**
- Create: `internal/webchat/authenticated_acceptance_test.go`
- Create: `docs/integrations/identity-authorization-rls.md`
- Create: `docs/evidence/2026-07-14-identity-authorization-rls.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Interfaces:**
- Consumes: frozen I01, identity CLI, runtime role grants, `serve`, authenticated handler, OpenClaw plugin token env, local PostgreSQL, and authenticated Grok CLI.
- Produces: deterministic I01-I05 proof and real I06 process-boundary evidence.

- [x] **Step 1: Write failing authenticated acceptance**

Issue client/operator tokens for A and B, drive the same anchor under both tenants, confirm A-only memory, prove B isolation, prove client role denial, revoke/expire tokens, omit tenant filters under the runtime role, attempt cross-tenant FK writes, and run concurrent pool reuse.

- [x] **Step 2: Verify RED then GREEN**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/webchat -run 'TestI01Authenticated' -v
```

- [x] **Step 3: Write deployment runbook**

Document migration, runtime role creation/grant, token issue/revoke, loopback/TLS serving, role matrix, OpenClaw token env, RLS verification, backup sensitivity for auth digests, and uninstall/revocation boundaries.

- [x] **Step 4: Run real authenticated OpenClaw/Grok replay**

Use isolated state and a dedicated database. Start `serve` with a non-owner runtime role, issue an OpenClaw client token, run one governed recall turn through `VERMORY_API_TOKEN`, inspect PostgreSQL, revoke the token, then prove the next OpenClaw turn remains available but has no successful Vermory lifecycle row.

- [x] **Step 5: Write evidence with deterministic/model separation**

Include exact versions, role attributes, policy inventory, filter-omission query results, pool reuse, token lifecycle counts, OpenClaw model route, revocation logs, and safe excerpts. Omit all real token values and digests.

- [x] **Step 6: Verify and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/webchat -run 'TestI01Authenticated' -v
git diff --check
git add internal/webchat/authenticated_acceptance_test.go docs README.md README.zh-CN.md
git commit -m "test: prove authenticated tenant isolation"
```

### Task 8: Release Verification, Push, And Draft PR Update

**Files:**
- Modify: `docs/superpowers/plans/2026-07-14-identity-authorization-rls.md`

- [x] **Step 1: Run release verification**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./...

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 \
  ./internal/authn ./internal/runtime ./internal/webchat \
  ./internal/identitycli ./internal/operatorcli ./cmd/vermory ./internal/provider

go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go build -o /tmp/vermory-identity-release ./cmd/vermory
PATH=/opt/homebrew/opt/node@24/bin:/opt/homebrew/bin:/usr/bin:/bin \
  pnpm -C integrations/openclaw check
PATH=/opt/homebrew/opt/node@24/bin:/opt/homebrew/bin:/usr/bin:/bin \
  pnpm -C integrations/openclaw pack --dry-run
git diff --check
```

- [x] **Step 2: Commit checked plan**

Mark all completed items only after fresh verification.

- [x] **Step 3: Push and update Draft PR 1**

Push `agent/grok-cli-runtime`. Update the Draft PR with the token boundary, role matrix, non-owner runtime role, RLS/filter-omission evidence, authenticated OpenClaw replay, and the next operations slice. Keep the overall goal active.
