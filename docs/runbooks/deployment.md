# Deployment Runbook

Full step-by-step deployment guide: **[deploy/runbooks/deploy.md](../../deploy/runbooks/deploy.md)**

This file is a quick-reference summary; the authoritative guide with RDS setup, security group config, and first-deploy walkthrough is in `deploy/runbooks/deploy.md`.

---

## Stack

EC2 (Docker Compose) + AWS RDS (PostgreSQL 16) + Redis (local container) + Caddy (TLS).

```bash
# Deploy / upgrade
cd deploy/environments/prod
docker compose up -d

# Migrations run automatically first (one-shot migrate service)
docker compose logs migrate         # check migration output

# Verify
curl https://id.qeet.in/healthz     # → {"status":"ok"}
curl https://id.qeet.in/readyz      # → {"status":"ok","db":"ok","redis":"ok"}
```

---

## Rolling update

```bash
nano .env                            # bump APP_IMAGE + MIGRATE_IMAGE to new tag X.Y.Z
docker compose pull migrate app
docker compose up migrate            # run migrations first, exits when done
docker compose up -d --no-deps app   # restart app only
docker compose logs -f app           # confirm clean startup
```

---

## Rollback

```bash
nano .env                            # revert APP_IMAGE to previous tag
docker compose up -d --no-deps app
```

> ⚠️ Never roll back a migration (`migrate down`). Fix forward with a new migration pair instead.

---

## Post-deployment checks

1. `curl https://id.qeet.in/healthz` returns `200 OK`
2. `curl https://id.qeet.in/readyz` returns `200 OK` (confirms DB + Redis)
3. `docker compose logs app --tail=20` — no `ERROR` lines at startup
4. Login flow works end-to-end (use a test account)
5. Check `GET /metrics` is reachable from your monitoring agent

---

## Useful commands

```bash
docker compose ps                    # container statuses
docker compose logs -f app           # tail app logs
docker compose logs -f caddy         # tail caddy/TLS logs
docker compose restart app           # restart without image change
docker compose pull && docker compose up -d   # pull latest pinned tags + restart
```
