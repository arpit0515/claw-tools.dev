#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
#  ClawTools Installer
#  Usage: ./install.sh
#  Or remotely: curl -fsSL https://clawtools.dev/install.sh | bash
# ─────────────────────────────────────────────────────────────────────────────
set -e

AMBER='\033[38;5;214m'
GREEN='\033[38;5;82m'
RED='\033[38;5;196m'
FAINT='\033[38;5;240m'
BOLD='\033[1m'
RESET='\033[0m'

# Resolve the repo root (works whether run as ./install.sh or piped from curl)
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || echo "$HOME/.claw/repo")"

echo ""
echo -e "${AMBER}  ╔══════════════════════════════════════════╗${RESET}"
echo -e "${AMBER}  ║  🦞  ${BOLD}ClawTools Installer${RESET}${AMBER}                 ║${RESET}"
echo -e "${AMBER}  ║  ${FAINT}MCP tool connectors for AI agents${RESET}${AMBER}       ║${RESET}"
echo -e "${AMBER}  ╚══════════════════════════════════════════╝${RESET}"
echo ""

# ── If piped from curl, clone the repo first ──────────────────────────────────
if [ ! -f "$REPO_DIR/tools.json" ]; then
  echo -e "${AMBER}  →  Cloning claw-tools.dev repo...${RESET}"
  REPO_DIR="$HOME/.claw/repo"
  if [ -d "$REPO_DIR/.git" ]; then
    git -C "$REPO_DIR" pull --quiet
  else
    git clone --quiet https://github.com/arpit0515/claw-tools.dev "$REPO_DIR"
  fi
  echo -e "${GREEN}  ✓  Repo ready at $REPO_DIR${RESET}"
  echo ""
fi

# ── Check Go ──────────────────────────────────────────────────────────────────
if ! command -v go &>/dev/null; then
  echo -e "${RED}  ✗  Go is not installed.${RESET}"
  echo -e "${FAINT}     Install from: https://go.dev/dl/${RESET}"
  echo ""
  exit 1
fi
GO_VER=$(go version | awk '{print $3}')
echo -e "${GREEN}  ✓  Go found${RESET} ${FAINT}($GO_VER)${RESET}"

# ── Check Git ─────────────────────────────────────────────────────────────────
if ! command -v git &>/dev/null; then
  echo -e "${RED}  ✗  Git is not installed.${RESET}"
  echo -e "${FAINT}     Install from: https://git-scm.com/${RESET}"
  echo ""
  exit 1
fi
GIT_VER=$(git --version | awk '{print $3}')
echo -e "${GREEN}  ✓  Git found${RESET} ${FAINT}($GIT_VER)${RESET}"
echo ""

# ── Build the Go TUI installer ────────────────────────────────────────────────
INSTALLER_SRC="$REPO_DIR/cmd/installer"
INSTALLER_BIN="/tmp/claw-installer-$$"

echo -e "${FAINT}  Building installer...${RESET}"
cd "$INSTALLER_SRC"
go build -o "$INSTALLER_BIN" . 2>&1
chmod +x "$INSTALLER_BIN"
echo -e "${GREEN}  ✓  Ready${RESET}"
echo ""

# Run, passing repo root so installer can find tools.json and tools/ folder
exec "$INSTALLER_BIN" --repo "$REPO_DIR" "$@"
