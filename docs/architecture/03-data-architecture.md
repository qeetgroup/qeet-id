# Data Architecture

## PostgreSQL multi-schema design

Qeet ID uses a **single PostgreSQL instance** with multiple schemas for bounded-context isolation. Schema-per-context ensures that tables from different domains don't bleed into each other and establishes a clean boundary for future service extraction.

| Schema | Owner context | Key tables |
|---|---|---|
| `tenant` | identity/organizations | `tenants`, `tenant_branding`, `tenant_domains` |
| `user` | identity/users | `users`, `groups`, `group_members`, `invitations` |
| `auth` | access, federation | `credentials`, `passkey_credentials`, `webauthn_sessions`, `mfa_secrets`, `sessions`, `oidc_clients`, `saml_connections`, `social_providers`, `api_keys`, `service_principals` |
| `rbac` | access/authorization | `roles`, `permissions`, `user_roles`, `role_permissions`, `relation_tuples` |
| `audit` | operations/audit | `audit_events` (hash-chained), `log_sinks` |
| `platform` | operations, developer | `outbox`, `outbox_dlq`, `notifications`, `billing_plans`, `billing_checkouts`, `agents`, `agent_credentials`, `vc_credentials`, `vc_revocations`, `secrets` |

> **Rule:** No cross-schema JOINs. If context A needs data owned by context B, it resolves it through a Go service call (interface-mediated), never by joining across schemas in SQL.

## Multi-tenancy

Every mutable table carries a `tenant_id` column. All repository queries are scoped to the tenant passed in the request context — there is no "global" view of users or resources.

```sql
-- Example: list users for a specific tenant only
SELECT id, email, display_name FROM "user".users
WHERE tenant_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC;
```

The `tenant_id` is extracted from the authenticated principal in `platform/httpx` and flows via `context.Context` through service → repository layers. Repositories **must not** run unscoped queries on tenant-owned tables.

## Migration strategy

Migrations live in [`migrations/`](../../migrations/) as golang-migrate SQL pairs (`NNNN_name.up.sql` / `NNNN_name.down.sql`).

**Rules:**
- Never edit an applied migration. Add a new pair instead.
- Migrations are sequential (0001–0062 as of pre-1.0).
- The production migration image is [`Dockerfile.migrate`](../../Dockerfile.migrate) — it copies the `migrations/` directory and runs `golang-migrate up` as a Kubernetes Job (see [`deploy/helm/qeet-id/templates/migration-job.yaml`](../../deploy/helm/qeet-id/templates/migration-job.yaml)).

Apply locally:
```bash
make migrate-up        # apply all pending
make migrate-down      # roll back one step (dev only)
make migrate-down-all  # roll back everything (dev only)
```

## Persistence layer

**Canonical path: hand-written SQL over pgx v5.**

Each domain follows the triplet pattern:
- `domain.go` — exported types and input structs
- `repository.go` — `*pgxpool.Pool`-backed persistence
- `http.go` — HTTP handler and route mounting

Repositories handle their own SQL. The `platform/dbutil` package provides shared helpers (`UpdateBuilder`, JSONB decode). The `platform/pgxerr` package maps PostgreSQL constraint errors to domain errors (`IsUnique`, `IsForeignKey`, etc.).

**sqlc:** `platform/sqlcgen/` is a generated template showing type-safe queries but is **not** the active path. Nothing imports it. Full sqlc adoption is a tracked future effort — don't introduce a sqlc/hand-written split within a domain.

## Transactional pattern

Every mutation that touches more than one table runs inside a single `pgx.Tx`:

```
business row  ┐
audit row      ├── single pgx.Tx (committed or rolled back together)
outbox row    ┘
```

The service layer owns the transaction. Handlers stay thin and never manage transactions directly. This ensures:
1. Mutations are atomic — partial writes never happen.
2. Audit events are never lost (they commit with the business row).
3. Outbox events are never duplicated or dropped relative to the business state.

## Key entity relationships

```
tenants (tenant schema)
  └─► users (user schema) [via tenant_id]
        ├─► user_roles (rbac schema) [M:N via roles]
        ├─► passkey_credentials (auth schema)
        ├─► mfa_secrets (auth schema)
        └─► audit_events (audit schema)

tenants
  ├─► oidc_clients (auth schema) — apps that use Qeet ID as IdP
  ├─► saml_connections (auth schema) — enterprise SSO connections
  ├─► api_keys (auth schema)
  ├─► agents (platform schema) — AI-agent definitions
  ├─► billing_plans (platform schema)
  └─► log_sinks (audit schema) — SIEM streaming config
```

## Soft deletes

Users and several other entities use soft deletes (`deleted_at IS NULL` filter). The `operations/retention` context runs a background worker that permanently purges soft-deleted records after the tenant-configured retention period, satisfying GDPR right-to-erasure obligations.

## JSONB usage

Some columns store structured data as PostgreSQL JSONB (e.g., tenant branding config, auth policy rules, OIDC client metadata). The `platform/dbutil` package provides a shared `DecodeJSONB` helper used across repositories.

## Connection pool

`platform/db` wraps a `pgxpool.Pool`. Configuration (max connections, idle timeout) comes from environment variables via `platform/config`. The `/readyz` probe issues a `pool.Ping()` to verify database connectivity before reporting healthy.
