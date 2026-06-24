# Deployment Topology

## Overview

Qeet ID deploys as a **single Go binary** in a Docker container, with PostgreSQL (AWS RDS) as the primary datastore and Redis for rate limiting. TLS is handled by Caddy (Let's Encrypt, automatic certificate renewal).

Current production topology: **EC2 + Docker Compose + AWS RDS**.

```
Internet
   │  443 / 80
   ▼
EC2 instance
   ├── Caddy         (TLS termination, reverse proxy)  ← ports 80 + 443
   ├── qeet-id app   (Go binary, distroless container)  ← :4001, internal only
   └── Redis         (rate limiting, ephemeral)         ← :6379, internal only

AWS RDS (PostgreSQL 16)   ← accessible only from EC2 security group
```

---

## Docker Compose stack

Config: [`deploy/environments/prod/docker-compose.yml`](../../deploy/environments/prod/docker-compose.yml)

| Service | Image | Purpose |
|:--------|:------|:--------|
| `migrate` | `ghcr.io/qeetgroup/qeet-id-migrate` | One-shot: runs `golang-migrate up` then exits |
| `app` | `ghcr.io/qeetgroup/qeet-id` | Go API server |
| `redis` | `redis:7-alpine` | Rate limiting (ephemeral — no backup needed) |
| `caddy` | `caddy:2-alpine` | TLS termination + reverse proxy |

Startup order: `migrate` completes successfully → `redis` healthy → `app` starts → `caddy` begins routing.

### Deploy / upgrade

```bash
cd deploy/environments/prod

# First deploy
docker compose up -d

# Rolling update (new image tag)
nano .env                          # update APP_IMAGE + MIGRATE_IMAGE to new tag
docker compose pull migrate app
docker compose up migrate          # run migrations first
docker compose up -d --no-deps app # restart app with new image
```

### Rollback

```bash
nano .env                          # revert APP_IMAGE to previous tag
docker compose up -d --no-deps app
```

> ⚠️ Never roll back a migration. If a migration has a bug, write a new one to fix it forward.

---

## Images

The Docker build context is the **repo root** (single Go module + `platform/database/migrations`):

```bash
docker build -f deploy/base/docker/Dockerfile          -t qeet-id:<tag> .
docker build -f deploy/base/docker/Dockerfile.migrate  -t qeet-id-migrate:<tag> .
```

| Image | Base | Notes |
|:------|:-----|:------|
| `qeet-id` | `gcr.io/distroless/static-debian12:nonroot` | No shell, nonroot user (65532), readonly FS |
| `qeet-id-migrate` | `migrate/migrate` | Copies only `platform/database/migrations/` |

Build metadata (version, commit SHA, Go version) is stamped via `-ldflags` into `platform/observability/buildinfo`.

CI/release publishes cosign-signed images with SBOM + provenance: `ghcr.io/qeetgroup/qeet-id` and `ghcr.io/qeetgroup/qeet-id-migrate`.

---

## Health probes

| Probe | Endpoint | Checks |
|:------|:---------|:-------|
| Liveness | `GET /healthz` | Process alive; returns build info JSON |
| Readiness | `GET /readyz` | Alive + `pgxpool.Ping()` + Redis ping |

---

## Frontend deployment

The three frontend apps are separate build artifacts deployed independently:

| App | Build output | Recommended hosting |
|:----|:-------------|:--------------------|
| `@qeetid/admin` (console) | `apps/console/dist/` (Vite SPA) | S3 + CloudFront, or Nginx on the same EC2 |
| `@qeetid/login` (hosted login) | Next.js SSR | Vercel, or Node container |
| `@qeetid/web` (website) | Next.js SSR | Vercel, or Node container |

Frontend builds: `pnpm build` (Turborepo runs all three in parallel). Node ≥ 20.9 required (`nvm use v22.20.0`).

---

## Observability

The Go API exposes:
- `GET /metrics` — Prometheus-compatible metrics
- `GET /healthz` — liveness
- `GET /readyz` — readiness (DB + Redis)
- Structured JSON logs to stdout
- Optional OTel tracing: set `OTEL_EXPORTER_OTLP_ENDPOINT` to enable (no-op when unset)

---

## Upgrade path

When ready to scale beyond a single server, Kubernetes (Helm) manifests, Terraform AWS modules, and a multi-environment staging setup are available in git history. See [ROADMAP.md](../../ROADMAP.md).
