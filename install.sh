#!/usr/bin/env bash
set -euo pipefail

REPO="VahanMargaryan/smtp-proxy"
INSTALL_DIR="/usr/local/bin"
ENV_FILE="/etc/default/smtp-proxy"
SERVICE_FILE="/etc/systemd/system/smtp-proxy.service"
SERVICE_USER="smtp-proxy"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  armv7l)  ARCH="armv7" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

BINARY="smtp-proxy-${OS}-${ARCH}"

echo "Installing smtp-proxy (${OS}/${ARCH})..."

# Get latest release tag
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
if [ -z "$LATEST" ]; then
  echo "Failed to fetch latest release"
  exit 1
fi
echo "Latest release: ${LATEST}"

# Download binary
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
echo "Downloading ${BINARY}..."
curl -fsSL -o "${TMP}/smtp-proxy" "https://github.com/${REPO}/releases/download/${LATEST}/${BINARY}"
chmod +x "${TMP}/smtp-proxy"

# Install binary
echo "Installing to ${INSTALL_DIR}/smtp-proxy..."
sudo install -m 755 "${TMP}/smtp-proxy" "${INSTALL_DIR}/smtp-proxy"

# Create system user
if ! id "$SERVICE_USER" &>/dev/null; then
  echo "Creating system user ${SERVICE_USER}..."
  sudo useradd -r -s /usr/sbin/nologin "$SERVICE_USER"
fi

# Create env file if it doesn't exist
if [ ! -f "$ENV_FILE" ]; then
  echo "Creating default config at ${ENV_FILE}..."
  sudo tee "$ENV_FILE" >/dev/null <<'EOF'
# SMTP Proxy Relay Configuration
# See https://github.com/VahanMargaryan/smtp-proxy for details

# --- Local Proxy Settings ---
SMTP_LISTEN_ADDR=:2525
SMTP_PROXY_USERNAME=proxyuser
SMTP_PROXY_PASSWORD=change-me-to-a-strong-password

# --- Destination SMTP Server ---
SMTP_DEST_HOST=smtp.example.com
SMTP_DEST_PORT=587
SMTP_DEST_USERNAME=user@example.com
SMTP_DEST_PASSWORD=your-smtp-password
SMTP_DEST_FROM=user@example.com

# --- Optional Settings ---
# SMTP_SERVER_DOMAIN=localhost
# SMTP_MAX_MESSAGE_SIZE=26214400
# LOG_LEVEL=info
EOF
  sudo chmod 600 "$ENV_FILE"
  sudo chown "$SERVICE_USER:$SERVICE_USER" "$ENV_FILE"
else
  echo "Config already exists at ${ENV_FILE}, skipping..."
fi

# Install systemd service
echo "Installing systemd service..."
curl -fsSL -o "${TMP}/smtp-proxy.service" "https://raw.githubusercontent.com/${REPO}/${LATEST}/smtp-proxy.service"
sudo install -m 644 "${TMP}/smtp-proxy.service" "$SERVICE_FILE"
sudo systemctl daemon-reload
sudo systemctl enable smtp-proxy

echo ""
echo "Installation complete!"
echo ""
echo "Next steps:"
echo "  1. Edit config:    sudo nano ${ENV_FILE}"
echo "  2. Start service:  sudo systemctl start smtp-proxy"
echo "  3. Check status:   sudo systemctl status smtp-proxy"
echo "  4. View logs:      sudo journalctl -u smtp-proxy -f"
