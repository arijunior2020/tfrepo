#!/usr/bin/env bash
set -euo pipefail

REPO="arijunior2020/tfrepo"
BINARY="tfrepo"
INSTALL_DIR="${TFREPO_INSTALL_DIR:-/usr/local/bin}"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux|darwin) ;;
  *)
    echo "Sistema operacional não suportado: $OS" >&2
    exit 1
    ;;
esac

# Detect arch
RAW_ARCH="$(uname -m)"
case "$RAW_ARCH" in
  x86_64|amd64)   ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)
    echo "Arquitetura não suportada: $RAW_ARCH" >&2
    exit 1
    ;;
esac

# Resolve latest version via GitHub API
echo "→ Buscando versão mais recente..."
VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' | head -1 | cut -d'"' -f4 | sed 's/^v//')"
if [ -z "$VERSION" ]; then
  echo "Não foi possível determinar a versão mais recente." >&2
  exit 1
fi
echo "→ Versão: v${VERSION}"

# Build URL
FILENAME="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${FILENAME}"

# Download to temp dir (cleaned up on exit)
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "→ Baixando ${URL}..."
curl -fsSL -o "${TMP}/${FILENAME}" "$URL"
tar xzf "${TMP}/${FILENAME}" -C "$TMP" "$BINARY"

# Install: sem sudo se possível, com sudo caso contrário
install_binary() {
  if [ -w "$INSTALL_DIR" ]; then
    mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  elif command -v sudo >/dev/null 2>&1; then
    echo "→ Instalando em ${INSTALL_DIR} (requer sudo)..."
    sudo mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
    sudo chmod +x "${INSTALL_DIR}/${BINARY}"
    return
  else
    return 1
  fi
  chmod +x "${INSTALL_DIR}/${BINARY}"
}

if ! install_binary 2>/dev/null; then
  # Fallback: ~/.local/bin (sem sudo)
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
  mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  chmod +x "${INSTALL_DIR}/${BINARY}"
  echo "→ Instalado em ${INSTALL_DIR} (sem permissão em /usr/local/bin)"
  echo "  Certifique-se de que ${INSTALL_DIR} está no seu PATH."
fi

echo ""
echo "✓ tfrepo instalado em ${INSTALL_DIR}/${BINARY}"
"${INSTALL_DIR}/${BINARY}" --version
