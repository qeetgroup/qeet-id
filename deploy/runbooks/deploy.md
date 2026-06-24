# Deployment Guide — EC2 + Docker Compose + AWS RDS

Simple production deployment: one EC2 instance running Docker Compose, with AWS RDS for PostgreSQL. No Kubernetes, no Terraform — just Docker and a `.env` file.

Database migrations run automatically on app startup (no separate step).

---

## Architecture

```
Internet
   │  443 / 80
   ▼
EC2 instance
   ├── Caddy (TLS, reverse proxy)  ← ports 80 + 443
   ├── qeet-id app container       ← port 4001 (internal only)
   └── Redis container             ← port 6379 (internal only)

AWS RDS (PostgreSQL 16)           ← only accessible from EC2 SG
```

---

## Step 1 — AWS RDS (PostgreSQL)

1. Open **AWS Console → RDS → Create database**
2. Engine: **PostgreSQL 16**, template: **Production** (or Free tier for testing)
3. Instance: `db.t3.micro` (start small, resize later)
4. DB name: `qeet_id` · Username: `qeetid` · set a strong password
5. **VPC**: same VPC as your EC2 · **Public access: No**
6. Note the **endpoint hostname** (e.g. `your-db.xxxx.us-east-1.rds.amazonaws.com`)

After creation, verify connectivity from your EC2:
```sql
psql "postgres://qeetid:PASSWORD@your-db.xxxx.rds.amazonaws.com:5432/qeet_id?sslmode=require" -c "SELECT version();"
```

---

## Step 2 — EC2 Instance

1. **Launch** an EC2 instance: Amazon Linux 2023 or Ubuntu 22.04
2. Instance type: `t3.small` minimum (1 vCPU, 2 GB RAM); `t3.medium` recommended
3. **Storage**: 20 GB gp3 root volume
4. **Elastic IP**: allocate one and associate it with the instance (static IP for DNS)
5. **Key pair**: download the `.pem` file for SSH access

---

## Step 3 — Security Groups

**EC2 Security Group** (inbound rules):

| Type  | Protocol | Port | Source    |
|:------|:---------|:-----|:----------|
| SSH   | TCP      | 22   | Your IP   |
| HTTP  | TCP      | 80   | 0.0.0.0/0 |
| HTTPS | TCP      | 443  | 0.0.0.0/0 |

**RDS Security Group** (inbound rules):

| Type       | Protocol | Port | Source              |
|:-----------|:---------|:-----|:--------------------|
| PostgreSQL | TCP      | 5432 | EC2 Security Group  |

> The RDS instance must **not** be publicly accessible. Only the EC2 can reach it.

---

## Step 4 — Bootstrap the EC2

```bash
# SSH into your EC2
ssh -i your-key.pem ec2-user@<ELASTIC-IP>

# Copy the setup script (from your local machine)
scp -i your-key.pem deploy/environments/prod/setup.sh ec2-user@<ELASTIC-IP>:~/

# Run it
bash ~/setup.sh

# Re-login so the docker group takes effect
exit && ssh -i your-key.pem ec2-user@<ELASTIC-IP>
```

---

## Step 5 — Generate Secrets

Run these **locally** (or on the EC2) and save the output — you'll need them in `.env`.

See [secrets.md](./secrets.md) for the full set of generation commands. Required values:

| Secret | Command |
|:-------|:--------|
| `JWT_SECRET` | `openssl rand -base64 48` |
| `JWT_SIGNING_KEY` | EC P-256 PEM — see secrets.md |
| `SECRETS_KEY` | `openssl rand -base64 32` |
| `CSRF_KEY` | `openssl rand -base64 32` |
| `SAML_IDP_KEY` + `SAML_IDP_CERT` | RSA PEM — see secrets.md |

---

## Step 6 — Configure `.env`

```bash
# On EC2 — create the working directory
mkdir -p /opt/qeet-id && cd /opt/qeet-id

# Copy files from your local machine
scp -i your-key.pem deploy/environments/prod/{docker-compose.yml,Caddyfile,.env.example} \
  ec2-user@<ELASTIC-IP>:/opt/qeet-id/

# Create .env from the example
cp .env.example .env
nano .env   # fill in DB_URL and all secret values
```

Key values to set in `.env`:

```bash
DB_URL=postgres://qeetid:PASSWORD@your-db.rds.amazonaws.com:5432/qeet_id?sslmode=require
JWT_SECRET=<generated>
JWT_SIGNING_KEY=<generated PEM, single line with \n>
SECRETS_KEY=<generated>
CSRF_KEY=<generated>
SAML_IDP_KEY=<generated PEM>
SAML_IDP_CERT=<generated PEM>
```

---

## Step 7 — Build and Deploy

Build the image directly on EC2 (no registry needed):

```bash
# On EC2 — clone the repo
git clone https://github.com/qeetgroup/qeet-id.git /opt/qeet-id-src
cd /opt/qeet-id-src

# Build the image
docker build -f deploy/base/docker/Dockerfile -t qeet-id:latest .

# Start the stack (migrations run automatically on first boot)
cd /opt/qeet-id
docker compose up -d

# Watch startup logs — migrations will print "migrations up to date" then server starts
docker compose logs -f app

# Verify the API is up
curl https://id.qeet.in/healthz        # → {"status":"ok"}
curl https://id.qeet.in/readyz         # → {"status":"ok","db":"ok","redis":"ok"}
```

If `app` fails to start: `docker compose logs app` — look for `migrations failed` (check `DB_URL`) or config validation errors (missing secrets).

---

## Updating (rolling deploy)

```bash
cd /opt/qeet-id-src

# Pull latest code
git pull

# Rebuild image
docker build -f deploy/base/docker/Dockerfile -t qeet-id:latest .

# Restart app (migrations run automatically on startup)
cd /opt/qeet-id
docker compose up -d --no-deps app
```

---

## Rollback

```bash
cd /opt/qeet-id-src

# Check out the previous release tag
git checkout vX.Y.Z

# Rebuild
docker build -f deploy/base/docker/Dockerfile -t qeet-id:latest .

# Restart app
cd /opt/qeet-id
docker compose up -d --no-deps app

docker compose logs -f app   # confirm it starts cleanly
```

> ⚠️ Never roll back a migration in production. If a migration has a bug, write a new migration to fix it forward.

---

## Useful commands

```bash
# View all container statuses
docker compose ps

# Tail logs
docker compose logs -f app
docker compose logs -f caddy

# Restart a single service
docker compose restart app

# Connect to RDS directly (no shell in distroless container)
docker run --rm -it --network qeet-id-prod_internal postgres:16-alpine \
  psql "$DB_URL"

# Check Redis
docker compose exec redis redis-cli ping

# Full stop (data volumes preserved)
docker compose down

# Full stop + wipe volumes (destructive — dev only)
docker compose down -v
```
