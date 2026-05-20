#!/usr/bin/env bash
set -euo pipefail

REPO="tadamo/podres"
BINARY="kubectl-podres"
INSTALL_DIR="/usr/local/bin"

# ---- detect OS ----
OS="$(uname -s)"
case "${OS}" in
  Linux*)  OS="linux" ;;
  Darwin*) OS="darwin" ;;
  *)
    echo "Unsupported OS: ${OS}" >&2
    exit 1
    ;;
esac

# ---- detect arch ----
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: ${ARCH}" >&2
    exit 1
    ;;
esac

# ---- resolve version ----
if [[ -z "${VERSION:-}" ]]; then
  VERSION="$(curl -sSfL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
fi

if [[ -z "${VERSION}" ]]; then
  echo "Could not determine latest release version." >&2
  exit 1
fi

echo "Installing ${BINARY} ${VERSION} (${OS}/${ARCH})..."

# ---- download ----
TARBALL="${BINARY}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "${TMPDIR}"' EXIT

curl -sSfL "${URL}" -o "${TMPDIR}/${TARBALL}"

# ---- verify checksum if available ----
CHECKSUM_URL="${URL}.sha256"
if curl -sSfL "${CHECKSUM_URL}" -o "${TMPDIR}/${TARBALL}.sha256" 2>/dev/null; then
  (cd "${TMPDIR}" && shasum -a 256 -c "${TARBALL}.sha256" --status) \
    || { echo "Checksum verification failed." >&2; exit 1; }
fi

# ---- extract ----
tar -xzf "${TMPDIR}/${TARBALL}" -C "${TMPDIR}"

# ---- install ----
if [[ -w "${INSTALL_DIR}" ]]; then
  install -m 0755 "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Installing to ${INSTALL_DIR} requires sudo..."
  sudo install -m 0755 "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

echo "${BINARY} ${VERSION} installed to ${INSTALL_DIR}/${BINARY}"
echo "Run: kubectl podres --help"
