#!/bin/bash
set -e

GITHUB_REPO="https://github.com/arpit0515/claw-setup-wizard"
INSTALL_DIR="$HOME/.picoclaw/wizard"

# ── Detect if piped from curl (no local repo) ─────────────────────────────────
# When piped via curl, BASH_SOURCE[0] is empty or /dev/stdin
IS_CURL_PIPE=false
if [ -z "${BASH_SOURCE[0]}" ] || [ "${BASH_SOURCE[0]}" = "/dev/stdin" ] || [ "${BASH_SOURCE[0]}" = "bash" ]; then
  IS_CURL_PIPE=true
fi

# If run locally from inside a git repo, use that directory
if [ "$IS_CURL_PIPE" = false ] && git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --is-inside-work-tree &>/dev/null; then
  REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
else
  REPO_DIR="$INSTALL_DIR"
fi

LOG_FILE="/tmp/claw-setup-install.log"

log() { echo "$1" | tee -a "$LOG_FILE"; }

log ""
log "🦞 claw-setup-wizard installer"
log "================================"
log "Started: $(date)"
log "Install dir: $REPO_DIR"

# ── (1) Get the source ────────────────────────────────────────────────────────
if [ "$IS_CURL_PIPE" = true ] || [ ! -d "$REPO_DIR/.git" ]; then
  # Fresh install via curl or missing repo
  if [ -d "$REPO_DIR/.git" ]; then
    log ""
    log "🔄 Pulling latest from GitHub..."
    BEFORE=$(git -C "$REPO_DIR" rev-parse HEAD)
    git -C "$REPO_DIR" fetch origin >> "$LOG_FILE" 2>&1
    git -C "$REPO_DIR" reset --hard origin/$(git -C "$REPO_DIR" rev-parse --abbrev-ref HEAD) >> "$LOG_FILE" 2>&1
    AFTER=$(git -C "$REPO_DIR" rev-parse HEAD)
    if [ "$BEFORE" != "$AFTER" ]; then
      log "✓ Updated to latest (${AFTER:0:7})"
    else
      log "✓ Already up to date (${AFTER:0:7})"
    fi
  else
    log ""
    log "📦 Cloning claw-setup-wizard..."
    mkdir -p "$REPO_DIR"
    git clone "$GITHUB_REPO" "$REPO_DIR" >> "$LOG_FILE" 2>&1
    log "✓ Cloned to $REPO_DIR"
  fi
else
  # Local run from inside git repo — pull latest
  log ""
  log "🔄 Pulling latest from GitHub..."
  BEFORE=$(git -C "$REPO_DIR" rev-parse HEAD)
  git -C "$REPO_DIR" fetch origin >> "$LOG_FILE" 2>&1
  git -C "$REPO_DIR" reset --hard origin/$(git -C "$REPO_DIR" rev-parse --abbrev-ref HEAD) >> "$LOG_FILE" 2>&1
  AFTER=$(git -C "$REPO_DIR" rev-parse HEAD)
  if [ "$BEFORE" != "$AFTER" ]; then
    log "✓ Updated to latest (${AFTER:0:7}) — restarting..."
    exec bash "$REPO_DIR/install.sh" "$@"
  else
    log "✓ Already up to date (${AFTER:0:7})"
  fi
fi

# ── (2) Build binary ──────────────────────────────────────────────────────────
BINARY="$REPO_DIR/claw-setup"

if [ ! -f "$BINARY" ]; then
  log ""
  log "🔨 Building claw-setup binary..."

  # Check for Go
  if ! command -v go &>/dev/null; then
    log ""
    log "📥 Go not found — installing Go 1.21..."
    ARCH=$(uname -m)
    case "$ARCH" in
      aarch64)  GO_ARCH="arm64" ;;
      armv7l|armv6l) GO_ARCH="armv6l" ;;
      x86_64)   GO_ARCH="amd64" ;;
      *)
        log "❌ Unsupported architecture: $ARCH"
        log "   Install Go manually from https://go.dev/dl/ then re-run this script."
        exit 1
        ;;
    esac
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    GO_VERSION="1.21.8"
    GO_TAR="go${GO_VERSION}.${OS}-${GO_ARCH}.tar.gz"
    GO_URL="https://go.dev/dl/${GO_TAR}"
    log "   Downloading $GO_URL ..."
    curl -fsSL "$GO_URL" -o "/tmp/$GO_TAR" >> "$LOG_FILE" 2>&1
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "/tmp/$GO_TAR" >> "$LOG_FILE" 2>&1
    rm "/tmp/$GO_TAR"
    export PATH=$PATH:/usr/local/go/bin
    log "✓ Go $(go version) installed"
  else
    log "✓ Go found: $(go version)"
  fi

  # Build
  export PATH=$PATH:/usr/local/go/bin
  cd "$REPO_DIR"
  go build -o claw-setup . >> "$LOG_FILE" 2>&1
  log "✓ Binary built: $BINARY"
else
  log "✓ Binary already exists — skipping build"
fi

chmod +x "$BINARY"

# ── (3) Autorun on boot (optional) ───────────────────────────────────────────
AUTORUN_MARKER="# claw-setup-autorun"

if grep -q "$AUTORUN_MARKER" ~/.bashrc 2>/dev/null; then
  log ""
  log "✓ Startup autorun already registered"
else
  log ""
  printf "🔁 Launch claw-setup automatically on boot? [y/N]: "
  read -r AUTORUN_ANSWER </dev/tty || AUTORUN_ANSWER="n"
  case "$AUTORUN_ANSWER" in
    [yY][eE][sS]|[yY])
      cat >> ~/.bashrc <<EOF

$AUTORUN_MARKER
if [ "\$(tty)" = "/dev/tty1" ]; then
  bash $BINARY
fi
EOF
      log "✓ Autorun registered in ~/.bashrc"

      if [[ "$(uname -s)" == "Linux" ]] && command -v systemctl &>/dev/null; then
        CURRENT_USER=$(whoami)
        AUTOLOGIN_CONF="/etc/systemd/system/getty@tty1.service.d/autologin.conf"
        if [ ! -f "$AUTOLOGIN_CONF" ]; then
          log "   Configuring auto-login for $CURRENT_USER on tty1..."
          sudo mkdir -p "$(dirname "$AUTOLOGIN_CONF")"
          sudo bash -c "cat > $AUTOLOGIN_CONF" <<EOF
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin $CURRENT_USER --noclear %I \$TERM
EOF
          sudo systemctl daemon-reload
          sudo systemctl restart getty@tty1
          log "✓ Auto-login configured for $CURRENT_USER"
        fi
      fi
      ;;
    *)
      log "⏭  Skipping autorun"
      ;;
  esac
fi

# ── (4) Add to PATH (optional) ────────────────────────────────────────────────
PATH_MARKER="# claw-setup-path"
if ! grep -q "$PATH_MARKER" ~/.bashrc 2>/dev/null; then
  cat >> ~/.bashrc <<EOF

$PATH_MARKER
export PATH="$REPO_DIR:\$PATH"
EOF
  log "✓ Added $REPO_DIR to PATH in ~/.bashrc"
  log "  Run: source ~/.bashrc  (or open a new terminal)"
fi

# ── (5) Start ─────────────────────────────────────────────────────────────────
LOCAL_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || ipconfig getifaddr en0 2>/dev/null || echo "localhost")

log ""
log "================================"
log "✅ Ready — open in your browser:"
log "   👉 http://$LOCAL_IP:3000"
log "================================"
log ""
log "Log saved to: $LOG_FILE"
log ""

exec "$BINARY"
