#!/usr/bin/env bash
set -euo pipefail

REPO="VahanMargaryan/smtp-proxy"
INSTALL_DIR="/usr/local/bin"
BINARY_PATH="${INSTALL_DIR}/smtp-proxy"
ENV_FILE="/etc/default/smtp-proxy"
SERVICE_FILE="/etc/systemd/system/smtp-proxy.service"
SERVICE_USER="smtp-proxy"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
RED='\033[0;31m'
NC='\033[0m'

info()    { echo -e "${CYAN}[*]${NC} $1"; }
success() { echo -e "${GREEN}[+]${NC} $1"; }
warn()    { echo -e "${YELLOW}[!]${NC} $1"; }
error()   { echo -e "${RED}[-]${NC} $1"; }

confirm() {
  local prompt="$1"
  local default="${2:-y}"
  local yn
  if [ "$default" = "y" ]; then
    read -rp "$(echo -e "${CYAN}[?]${NC} ${prompt} [Y/n]: ")" yn
    yn="${yn:-y}"
  else
    read -rp "$(echo -e "${CYAN}[?]${NC} ${prompt} [y/N]: ")" yn
    yn="${yn:-n}"
  fi
  [[ "$yn" =~ ^[Yy]$ ]]
}

prompt_value() {
  local prompt="$1"
  local default="$2"
  local value
  read -rp "$(echo -e "${CYAN}[?]${NC} ${prompt} [${default}]: ")" value
  echo "${value:-$default}"
}

prompt_secret() {
  local prompt="$1"
  local default="$2"
  local value
  read -srp "$(echo -e "${CYAN}[?]${NC} ${prompt} [${default}]: ")" value
  echo ""
  echo "${value:-$default}"
}

echo ""
echo -e "${GREEN}smtp-proxy installer${NC}"
echo "────────────────────"
echo ""

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  armv7l)  ARCH="armv7" ;;
  *) error "Unsupported architecture: $ARCH"; exit 1 ;;
esac

BINARY_NAME="smtp-proxy-${OS}-${ARCH}"
info "Detected platform: ${OS}/${ARCH}"

# Get latest release tag
info "Checking latest release..."
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
if [ -z "$LATEST" ]; then
  error "Failed to fetch latest release"
  exit 1
fi

# Check if already installed and compare versions
CURRENT=""
if [ -f "$BINARY_PATH" ]; then
  CURRENT=$("$BINARY_PATH" -version 2>/dev/null || echo "unknown")
fi

if [ -n "$CURRENT" ] && [ "$CURRENT" != "unknown" ]; then
  if [ "$CURRENT" = "$LATEST" ]; then
    success "smtp-proxy is already up to date (${CURRENT})"
    if ! confirm "Reinstall anyway?" "n"; then
      echo ""
      info "Nothing to do."
      exit 0
    fi
  else
    warn "Installed version: ${CURRENT}"
    info "Available version: ${LATEST}"
    echo ""
    if ! confirm "Update smtp-proxy to ${LATEST}?"; then
      exit 0
    fi
  fi
elif [ -f "$BINARY_PATH" ]; then
  warn "smtp-proxy is installed (version unknown)"
  info "Available version: ${LATEST}"
  echo ""
  if ! confirm "Update smtp-proxy to ${LATEST}?"; then
    exit 0
  fi
else
  info "Available version: ${LATEST}"
  echo ""
  if ! confirm "Install smtp-proxy ${LATEST}?"; then
    exit 0
  fi
fi

echo ""

# Download binary
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
info "Downloading ${BINARY_NAME}..."
curl -fsSL -o "${TMP}/smtp-proxy" "https://github.com/${REPO}/releases/download/${LATEST}/${BINARY_NAME}"
chmod +x "${TMP}/smtp-proxy"
success "Downloaded successfully"

# Stop service if running (update scenario)
WAS_RUNNING=false
if systemctl is-active --quiet smtp-proxy 2>/dev/null; then
  WAS_RUNNING=true
  info "Stopping smtp-proxy service..."
  sudo systemctl stop smtp-proxy
fi

# Install binary
info "Installing binary to ${BINARY_PATH}..."
sudo install -m 755 "${TMP}/smtp-proxy" "$BINARY_PATH"
success "Binary installed"

# Create system user
if ! id "$SERVICE_USER" &>/dev/null; then
  info "Creating system user '${SERVICE_USER}'..."
  sudo useradd -r -s /usr/sbin/nologin "$SERVICE_USER"
  success "User created"
fi

# Configuration
echo ""
if [ -f "$ENV_FILE" ]; then
  success "Config exists at ${ENV_FILE} — keeping current settings"
else
  info "No config found. Let's set up your SMTP proxy."
  echo ""

  LISTEN_ADDR=$(prompt_value "Listen address" ":2525")
  PROXY_USER=$(prompt_value "Proxy username (clients authenticate with this)" "proxyuser")
  PROXY_PASS=$(prompt_secret "Proxy password" "change-me")
  echo ""
  DEST_HOST=$(prompt_value "Upstream SMTP host" "smtp.example.com")
  DEST_PORT=$(prompt_value "Upstream SMTP port" "587")
  DEST_USER=$(prompt_value "Upstream SMTP username" "user@example.com")
  DEST_PASS=$(prompt_secret "Upstream SMTP password" "")
  DEST_FROM=$(prompt_value "Envelope sender address" "$DEST_USER")

  echo ""
  info "Writing config to ${ENV_FILE}..."
  sudo tee "$ENV_FILE" >/dev/null <<EOF
# SMTP Proxy Relay Configuration
# See https://github.com/VahanMargaryan/smtp-proxy for details

# --- Local Proxy Settings ---
SMTP_LISTEN_ADDR=${LISTEN_ADDR}
SMTP_PROXY_USERNAME=${PROXY_USER}
SMTP_PROXY_PASSWORD=${PROXY_PASS}

# --- Destination SMTP Server ---
SMTP_DEST_HOST=${DEST_HOST}
SMTP_DEST_PORT=${DEST_PORT}
SMTP_DEST_USERNAME=${DEST_USER}
SMTP_DEST_PASSWORD=${DEST_PASS}
SMTP_DEST_FROM=${DEST_FROM}

# --- Optional Settings ---
# SMTP_SERVER_DOMAIN=localhost
# SMTP_MAX_MESSAGE_SIZE=26214400
# LOG_LEVEL=info
EOF
  sudo chmod 600 "$ENV_FILE"
  sudo chown "$SERVICE_USER:$SERVICE_USER" "$ENV_FILE"
  success "Config written"
fi

# Install/update systemd service
echo ""
info "Installing systemd service..."
curl -fsSL -o "${TMP}/smtp-proxy.service" "https://raw.githubusercontent.com/${REPO}/${LATEST}/smtp-proxy.service"
sudo install -m 644 "${TMP}/smtp-proxy.service" "$SERVICE_FILE"
sudo systemctl daemon-reload
sudo systemctl enable smtp-proxy
success "Service installed and enabled"

# Restart if it was running before update
if [ "$WAS_RUNNING" = true ]; then
  echo ""
  if confirm "smtp-proxy was running before update. Restart now?"; then
    sudo systemctl start smtp-proxy
    success "Service restarted"
  else
    warn "Service is stopped. Start manually: sudo systemctl start smtp-proxy"
  fi
else
  echo ""
  if confirm "Start smtp-proxy now?"; then
    sudo systemctl start smtp-proxy
    sleep 1
    if systemctl is-active --quiet smtp-proxy; then
      success "Service is running"
    else
      error "Service failed to start. Check logs: sudo journalctl -u smtp-proxy -e"
    fi
  fi
fi

echo ""
echo -e "${GREEN}All done!${NC}"
echo ""
echo "  Config:  sudo nano ${ENV_FILE}"
echo "  Status:  sudo systemctl status smtp-proxy"
echo "  Logs:    sudo journalctl -u smtp-proxy -f"
echo "  Restart: sudo systemctl restart smtp-proxy"
echo ""
