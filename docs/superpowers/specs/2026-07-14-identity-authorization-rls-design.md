# Identity, Authorization, And PostgreSQL RLS Design

Date: 2026-07-14

Status: approved for implementation by the active Vermory execution goal

## Goal

Add the first self-hostable multi-tenant serving boundary without weakening the existing local single-tenant workflows. An authenticated request must derive its tenant and role from a server-issued credential, not from request JSON, URL paths, OpenClaw plugin configuration, or model output. PostgreSQL must enforce the same boundary independently of application query filters.

## Product Boundary

Vermory has two deliberate serving profiles:

### Local Profile

The existing `vermory web-chat --tenant-id ...` and `vermory mcp-stdio --tenant-id ...` commands remain local operator/development entry points. They are loopback-oriented, server-configured, and are not advertised as remotely safe hosted services.

### Authenticated Profile

The new `vermory serve` command exposes the existing conversation, governance, Global Defaults, bridge, and OpenClaw routes behind bearer-token authentication. It derives a principal from the token and constructs tenant-scoped services for that request.

The authenticated profile:

- binds every request to exactly one server-known tenant;
- authorizes route families by role;
- connects application queries through a non-owner PostgreSQL runtime role;
- sets a transaction/connection tenant context before any tenant query;
- relies on PostgreSQL RLS as a second barrier when a query omits a tenant predicate;
- refuses non-loopback listeners unless TLS certificate and key are configured;
- does not implement user registration, OIDC, SSO, billing, management UI, or a public token issuance endpoint.

Admin database access is used only for migrations and token lifecycle commands. It is never used as the serving connection for authenticated tenant requests.

## User-Visible Contract

An operator creates a token once:

```text
vmt_<public-token-id>_<secret>
```

The secret is displayed once and is never stored in PostgreSQL, logs, receipts, error responses, or the repository. PostgreSQL stores only a SHA-256 digest of the complete token plus non-secret metadata.

An authenticated client sends:

```http
Authorization: Bearer vmt_<public-token-id>_<secret>
```

The client does not send `tenant_id`, `continuity_id`, `channel authority`, role, lifecycle state, or provider selection as an authority claim. The server maps the credential to:

```text
principal = { tenant_id, subject_id, role, token_id, expires_at }
```

If a request contains an authority field that the route does not accept, the existing strict JSON decoder rejects it. If the token is malformed, unknown, revoked, or expired, the response is a bounded `401` without token, tenant, database, or model details.

## Roles And Route Authorization

V1 has three fixed roles. Roles are assigned at token issuance and are not accepted from requests.

| Role | Allowed behavior |
|---|---|
| `client` | Prepare/complete/fail conversation turns, including OpenClaw lifecycle routes; no governance inspection or mutation |
| `operator` | Client behavior plus conversation inspection, memory confirmation/correction/forget, Global Defaults, and bridge operations |
| `owner` | Operator behavior; token lifecycle remains an admin CLI operation, not an HTTP mutation |

The route policy is:

- `POST /v1/chat/turn`: `client` or higher;
- `POST /v1/integrations/openclaw/turns/{prepare,complete,fail}`: `client` or higher;
- `GET /v1/conversations/inspect`: `operator` or higher;
- `GET /v1/defaults`: `operator` or higher;
- `/v1/memories/*`, `/v1/defaults/*`, `/v1/bridges/*`: `operator` or higher.

Authorization failures do not reveal whether a target memory, bridge, or continuity exists in another tenant. The handler resolves services with the authenticated tenant only after authorization succeeds.

## Token Lifecycle

The admin CLI provides:

```text
vermory identity token issue
vermory identity token inspect
vermory identity token revoke
```

Required issue inputs are admin database URL, tenant ID, subject ID, role, and optional expiry. The CLI validates tenant and role lengths, generates cryptographically secure public ID and secret bytes, writes one digest row, and prints the complete token once. Repeating an operation ID is idempotent; reusing it with different token parameters is rejected.

The token table lives in a restricted `vermory_auth` schema. The application runtime role cannot select the table. It can execute one security-definer lookup function that receives `(public_token_id, token_digest)` and returns only active principal metadata. The function uses a fixed catalog-only search path and rejects revoked or expired credentials.

Revocation takes effect on the next request. Existing in-flight requests are not retroactively cancelled; their database transaction still has one tenant context and one authorization decision.

## Database Roles And Connection Boundary

Deployments use two database identities:

### Admin/Migration Role

Owns migrations, restricted auth tables, and token lifecycle operations. It is never used by `serve` for tenant data queries.

### Runtime Role

The runtime role has `LOGIN`, does not own Vermory tables, does not have `BYPASSRLS`, and receives privileges only on the authenticated continuity tables plus `EXECUTE` on the token lookup function. It receives no privileges on the legacy project/source/capsule tables until those tables gain a tenant model in a later slice.

The runtime store uses a pool acquire/release boundary:

1. every tenant-bearing store method places its server-derived tenant in the context;
2. `BeforeAcquire` sets `vermory.tenant_id` on the acquired connection;
3. PostgreSQL RLS policies compare every row's `tenant_id` with that setting;
4. `AfterRelease` clears the setting before the connection returns to the pool;
5. a missing tenant context cannot acquire a usable runtime connection.

The application still keeps explicit tenant predicates and service-owned tenant IDs. RLS is a second barrier, not a replacement for application scoping.

## RLS Scope

Migration `00009_identity_authorization_rls.sql` enables RLS on all currently served tenant-bearing tables:

- `continuity_spaces`;
- `continuity_bindings`;
- `conversation_bindings`;
- `observations`;
- `governed_memories`;
- `memory_deliveries`;
- `memory_search_documents`;
- `conversation_turns`;
- `bridge_operations`;
- `bridge_events`;
- `bridge_memory_effects`;
- `conversation_links`.

Each table receives tenant isolation policies for `SELECT`, `INSERT`, `UPDATE`, and `DELETE` as appropriate. Policies use `current_setting('vermory.tenant_id', true)` and never trust request input.

The migration also adds tenant-aware unique keys and composite foreign keys for the served graph. A row from tenant A cannot reference a continuity, observation, delivery, bridge, or memory owned by tenant B even if an attacker knows the UUID. Existing single-column primary keys remain for application identity; the composite keys enforce tenant ownership at the relational boundary.

The older project/source/capsule/WCEF tables are intentionally not granted to the runtime role in this slice. Their later tenant migration is a separate design decision, not an accidental RLS hole in the authenticated serving path.

## Request Flow

```text
HTTP request
  -> bearer parser
  -> restricted token lookup
  -> principal {tenant, subject, role}
  -> route scope check
  -> tenant-scoped service facade
  -> store context tenant
  -> pooled connection sets vermory.tenant_id
  -> PostgreSQL RLS + explicit tenant predicates
  -> semantic delivery / governed mutation
```

OpenClaw integration uses the same boundary. The plugin reads an operator-provided `VERMORY_API_TOKEN` environment variable when present and sends it as a bearer token. The token value is not a plugin config field, not included in error messages, and not passed to the model. Local unauthenticated loopback mode remains compatible when no token is configured.

## Failure Semantics

- missing or invalid bearer token: `401`;
- valid token with insufficient role: `403`;
- valid token whose tenant cannot see a resource: indistinguishable not-found/forbidden response with no cross-tenant details;
- missing tenant context on runtime connection: fail closed;
- RLS denial: mapped to a safe service error, never retried as admin;
- auth database unavailable: authenticated request fails closed; no anonymous fallback;
- Vermory unavailable to OpenClaw: chat remains fail-open exactly as in the local plugin contract, but no persistence success is reported;
- migration role unavailable: `serve` does not run migrations implicitly and reports an operator setup error.

## Acceptance Cases

### I01 Token Lifecycle

Issue two tenant-scoped tokens, authenticate both, inspect metadata without exposing secret material, reject malformed/expired/revoked tokens, and prove revocation on the next request.

### I02 HTTP Tenant Isolation

Tenant A confirms a memory and receives it in a fresh delivery. Tenant B uses its own valid token against the same channel/thread and receives no A data. Supplying A's tenant ID, continuity ID, or memory ID in B's JSON is rejected or returns safe not-found behavior.

### I03 Role Authorization

The client role can complete a turn but cannot inspect or mutate governed memory, defaults, or bridges. The operator role can perform those explicit governance operations. No role can select a different tenant.

### I04 Direct RLS Filter-Omission Attack

Under the non-owner runtime role, set tenant context A and execute read queries without tenant predicates. Only A rows are visible. Repeat under B. With no tenant context, no tenant rows are visible. Attempt cross-tenant inserts and foreign-key references; PostgreSQL rejects them.

### I05 Pool Reuse And Stale Context

Run A, B, A, and B requests through a small pool with concurrent execution. Every delivery remains tenant-correct. A released connection never carries A's tenant setting into B.

### I06 Authenticated OpenClaw Replay

Run one real OpenClaw/Grok turn through `serve` with `VERMORY_API_TOKEN`, confirm the model-facing packet and PostgreSQL receipt, revoke the token, and prove the next prepare fails while OpenClaw still returns a model answer without claiming persistence.

## Non-Goals

- OIDC, SAML, external identity-provider discovery, user registration, password login, or browser sessions;
- HTTP token issuance or tenant creation;
- management UI or dashboard;
- public cloud control plane;
- automatic tenant linking or cross-tenant bridges;
- silent fallback from authenticated serving to the admin database role;
- claiming the old project/source/capsule tables are remotely multi-tenant before their own migration.

## Completion Gate

This slice is complete only when I01-I06 have frozen fixtures, deterministic tests, a real authenticated OpenClaw/Grok replay, RLS filter-omission evidence under a non-owner role, migration/reopen evidence, clean release verification, a committed evidence report, and a GitHub Draft PR update. Mock-only authentication tests do not satisfy the gate.
