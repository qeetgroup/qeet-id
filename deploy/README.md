# deploy/

Deployment artifacts for Qeet ID. Current target: **EC2 + Docker Compose + AWS RDS**.

```
deploy/
├── base/
│   └── docker/              Dockerfile (app) + Dockerfile.migrate + build.sh
├── environments/
│   ├── dev/                 Local dev — Postgres-only Compose (used by `make db-up`)
│   └── prod/
│       └── compose/         Production stack (app + migrate + redis + caddy)
│           ├── docker-compose.yml
│           ├── .env.example
│           ├── Caddyfile
│           └── setup.sh     One-shot EC2 bootstrap script
└── runbooks/
    ├── deploy.md            Step-by-step first-deploy guide (start here)
    └── secrets.md           Secret generation commands
```

## Quick start

```bash
# 1. Bootstrap a fresh EC2 instance
bash deploy/environments/prod/setup.sh

# 2. Copy compose files to EC2 and fill in .env
cp deploy/environments/prod/.env.example .env
# edit .env — set DB_URL (RDS), image tags, all secrets

# 3. Deploy
docker compose -f deploy/environments/prod/docker-compose.yml up -d

# 4. Verify
curl https://id.qeet.in/healthz
```

Full step-by-step walkthrough (RDS setup, security groups, secrets generation): **[runbooks/deploy.md](runbooks/deploy.md)**

## Build images

The build context must be the **repo root** (Go module + migrations live there):

```bash
docker build -f deploy/base/docker/Dockerfile         -t qeet-id:dev .
docker build -f deploy/base/docker/Dockerfile.migrate -t qeet-id-migrate:dev .

# Or use the helper script
./deploy/base/docker/build.sh dev
```

CI/release publishes cosign-signed images: `ghcr.io/qeetgroup/qeet-id` and `ghcr.io/qeetgroup/qeet-id-migrate`.

## Upgrade path

When you're ready to scale beyond a single server:

- **Kubernetes + Helm**: chart, base manifests, and per-env values are available in git history — restore them when needed.
- **Terraform**: AWS infrastructure modules (RDS, ECR, KMS, Secrets Manager) also in git history.
- Planned packaging is tracked in [ROADMAP.md](../ROADMAP.md).
