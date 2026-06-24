# Docker Build

The image definition lives **here** (`deploy/base/docker/`), but the **build
context must be the repository root** — the build needs the whole Go module and
the migration SQL files embedded at compile time (`platform/database/migrations/`).

| File | Image | Purpose |
|---|---|---|
| `Dockerfile` | `ghcr.io/qeetgroup/qeet-id` | Distroless API server (migrations embedded) |
| `../../../.dockerignore` | — | At the repo root (Docker reads it only from the context root) |

## Build image locally

```bash
# API server (context = repo root)
docker build -f deploy/base/docker/Dockerfile -t qeet-id:latest .

# Or use the helper script
./deploy/base/docker/build.sh dev        # tagged :dev
./deploy/base/docker/build.sh v1.2.3     # tagged :v1.2.3
```

## Image architecture

**`Dockerfile`:**
- Stage 1: `golang:1.25-alpine` — compiles the binary with `-ldflags` stamping (version, commit, date); migration SQL files are embedded via `//go:embed`
- Stage 2: `gcr.io/distroless/static` — minimal runtime; no shell, no package manager
- Runs as non-root user (65532)
- Exposes port 4001
- Migrations run automatically at startup — no separate migration image or step

## CI/CD

Images are built and pushed by `.github/workflows/release.yml`:
1. Triggered by a `vX.Y.Z` tag (created by release-please)
2. Builds the app image
3. Signs with `cosign` keyless signing (Sigstore)
4. Attaches SBOM + provenance attestations
5. Pushes to `ghcr.io/qeetgroup/qeet-id`

## Verify a signed image before promoting

```bash
cosign verify ghcr.io/qeetgroup/qeet-id:X.Y.Z \
  --certificate-identity-regexp 'https://github.com/qeetgroup/qeet-id/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Environment variables

All configuration is provided via environment variables at runtime. See `platform/config/config.go` for the full list. Required production variables (the server refuses to start without them outside `SERVICE_ENV=dev`):

- `DB_URL` — PostgreSQL connection string
- `JWT_SIGNING_KEY` — EC P-256 private key (PEM)
- `JWT_SECRET` — ≥32-char HMAC secret
- `APP_BASE_URL` — Public HTTPS base URL
- `ALLOWED_ORIGINS` — Comma-separated allowed CORS origins
- `CSRF_KEY` — 32-byte CSRF HMAC key

See [`../../runbooks/secrets.md`](../../runbooks/secrets.md) for secret generation commands.
