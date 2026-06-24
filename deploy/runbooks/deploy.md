# Deployment Guide — EC2 + Docker Compose + AWS RDS

Simple production deployment: one EC2 instance running Docker Compose, with AWS RDS for PostgreSQL. No Kubernetes, no Terraform — just Docker and a `.env` file.

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

After creation, connect and run:
```sql
-- verify connectivity from your EC2 (after security group step below)
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
nano .env   # fill in DB_URL, all secret values, image tags
```

Key values to set in `.env`:

```bash
APP_IMAGE=ghcr.io/qeetgroup/qeet-id:X.Y.Z       # latest release tag
MIGRATE_IMAGE=ghcr.io/qeetgroup/qeet-id-migrate:X.Y.Z
DB_URL=postgres://qeetid:PASSWORD@your-db.rds.amazonaws.com:5432/qeet_id?sslmode=require
JWT_SECRET=<generated>
JWT_SIGNING_KEY=<generated PEM, single line with \n>
SECRETS_KEY=<generated>
CSRF_KEY=<generated>
SAML_IDP_KEY=<generated PEM>
SAML_IDP_CERT=<generated PEM>
```

---

## Step 7 — First Deploy

```bash
cd /opt/qeet-id

# Authenticate with GitHub Container Registry
echo $GITHUB_PAT | docker login ghcr.io -u <YOUR-GITHUB-USERNAME> --password-stdin

# Pull images
docker compose pull

# Start (migrate runs first, then app)
docker compose up -d

# Watch migration logs — must complete successfully before app starts
docker compose logs -f migrate

# Verify the API is up
curl https://id.qeet.in/healthz        # → {"status":"ok"}
curl https://id.qeet.in/readyz         # → {"status":"ok","db":"ok","redis":"ok"}
```

If `migrate` fails: check `DB_URL` is correct and the RDS security group allows port 5432 from your EC2.

If `app` fails to start: `docker compose logs app` — look for `config:` validation errors (missing secrets).

---

## Updating (rolling deploy)

```bash
cd /opt/qeet-id

# 1. Update image tags in .env
nano .env   # set APP_IMAGE and MIGRATE_IMAGE to new tag X.Y.Z+1

# 2. Pull new images
docker compose pull migrate app

# 3. Run migrations first (one-shot, exits when done)
docker compose up migrate

# 4. Restart app with new image (zero interruption for most changes)
docker compose up -d --no-deps app
```

---

## Rollback

```bash
cd /opt/qeet-id

# Change image tags back to the previous version in .env
nano .env   # set APP_IMAGE + MIGRATE_IMAGE back to X.Y.Z

# Restart app only (do NOT re-run migrate backward — use roll-forward instead)
docker compose up -d --no-deps app

docker compose logs -f app   # confirm it starts cleanly
```

> ⚠️ Never roll back a migration with `down` in production. If a migration has a bug, write a new migration to fix it forward.

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

# Open a shell in the app container (distroless — no shell available)
# Connect to RDS directly instead:
docker run --rm -it --network qeet-id-prod_internal postgres:16-alpine \
  psql "$DB_URL"

# Check Redis
docker compose exec redis redis-cli ping

# Full stop (data volumes preserved)
docker compose down

# Full stop + wipe volumes (destructive — dev only)
docker compose down -v
```
