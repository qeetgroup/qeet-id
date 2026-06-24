#!/usr/bin/env bash
# setup.sh — one-shot EC2 bootstrap for Qeet ID (Amazon Linux 2 or Ubuntu)
# Run once on a fresh instance: bash setup.sh
set -euo pipefail

echo "==> Detecting OS..."
if [ -f /etc/os-release ]; then
  . /etc/os-release
  OS=$ID
else
  echo "Cannot detect OS. Exiting." && exit 1
fi

echo "==> Installing Docker..."
if [[ "$OS" == "amzn" ]]; then
  sudo yum update -y
  sudo yum install -y docker
  sudo systemctl enable --now docker
elif [[ "$OS" == "ubuntu" ]]; then
  sudo apt-get update -y
  sudo apt-get install -y ca-certificates curl gnupg
  sudo install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
    | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
    https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
    | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
  sudo apt-get update -y
  sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
  sudo systemctl enable --now docker
else
  echo "Unsupported OS: $OS. Install Docker manually." && exit 1
fi

echo "==> Adding $USER to docker group (re-login required after this)..."
sudo usermod -aG docker "$USER"

echo "==> Installing Docker Compose plugin (if not already present)..."
if ! docker compose version &>/dev/null 2>&1; then
  COMPOSE_VERSION="v2.27.0"
  sudo mkdir -p /usr/local/lib/docker/cli-plugins
  sudo curl -SL "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-linux-$(uname -m)" \
    -o /usr/local/lib/docker/cli-plugins/docker-compose
  sudo chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
fi
docker compose version

echo "==> Creating /opt/qeet-id working directory..."
sudo mkdir -p /opt/qeet-id
sudo chown "$USER":"$USER" /opt/qeet-id

echo ""
echo "✅  Setup complete!"
echo ""
echo "Next steps:"
echo "  1. Re-login (or run: newgrp docker) so the docker group takes effect."
echo "  2. Copy deploy files to /opt/qeet-id:"
echo "       scp deploy/environments/prod/* ec2-user@<IP>:/opt/qeet-id/"
echo "  3. Generate secrets (see deploy/runbooks/secrets.md) and fill in .env"
echo "  4. Authenticate with GitHub Container Registry:"
echo "       echo \$GITHUB_PAT | docker login ghcr.io -u <USERNAME> --password-stdin"
echo "  5. First deploy:"
echo "       cd /opt/qeet-id && docker compose up -d"
echo "  6. Check migration logs:"
echo "       docker compose logs migrate"
echo "  7. Verify the API is healthy:"
echo "       curl https://id.qeet.in/healthz"
