#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if [[ "${EUID}" -eq 0 ]]; then
  SUDO=""
else
  if ! command -v sudo >/dev/null 2>&1; then
    echo "This installer requires sudo. Please install sudo and rerun it." >&2
    exit 1
  fi
  SUDO="sudo"
fi

if [[ ! -f /etc/os-release ]]; then
  echo "Unsupported system: /etc/os-release not found." >&2
  exit 1
fi

. /etc/os-release
if [[ "$ID" != "debian" ]]; then
  echo "This installer is intended for Debian-based systems. Detected: ${ID:-unknown}" >&2
  exit 1
fi

echo ""
echo "========================================="
echo " dcomms installer for Debian"
echo "========================================="
echo "This script will install Docker and Go, then build the local dcomms binary."
echo ""

echo "[1/5] Installing base packages..."
$SUDO apt-get update
$SUDO apt-get install -y ca-certificates curl gnupg golang

echo "[2/5] Configuring Docker repository..."
if [[ ! -f /etc/apt/keyrings/docker.asc ]]; then
  $SUDO install -m 0755 -d /etc/apt/keyrings
  $SUDO curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
  $SUDO chmod a+r /etc/apt/keyrings/docker.asc

  $SUDO tee /etc/apt/sources.list.d/docker.sources >/dev/null <<EOF2
Types: deb
URIs: https://download.docker.com/linux/debian
Suites: ${VERSION_CODENAME}
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF2
fi

$SUDO apt-get update

echo "[3/5] Installing Docker..."
$SUDO apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

if ! id -nG "$USER" | tr ' ' '\n' | grep -qx 'docker'; then
  echo "[4/5] Adding your user to the Docker group..."
  $SUDO usermod -aG docker "$USER"
  echo "Note: your Docker group membership was updated. Please log out and back in before using Docker without sudo."
else
  echo "[4/5] Docker group already configured for this user."
fi

$SUDO apt-get autoremove -y

echo "[5/5] Building dcomms binary..."
mkdir -p service

go build -o service/dcommsctl ./...

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker command is not available in PATH after installation." >&2
  exit 1
fi

echo ""
echo "Validation results:"
docker --version
docker compose version
go version

echo ""
echo "Setup complete."
echo "Binary built at: ${SCRIPT_DIR}/service/dcommsctl"

echo "Next step: log out and back in if Docker permissions changed, then run the app from this repo."

# TODO: discuss whether we should keep the build output in service or move it to a more explicit folder
# such as bin/ or dist/ for clarity and easier release packaging.
# NOTE: service is intentionally kept as-is for now to avoid breaking the current workflow.
