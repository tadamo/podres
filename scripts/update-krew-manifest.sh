#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:?Usage: $0 <version tag, e.g. v1.0.1>}"
REPO="tadamo/podres"
MANIFEST="plugins/kubectl-podres.yaml"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading checksums for $VERSION..."
gh release download "$VERSION" --repo "$REPO" --pattern "*.sha256" --dir "$TMPDIR"

sha() { awk '{print $1}' "$TMPDIR/${1}.sha256"; }

LINUX_AMD64=$(sha "kubectl-podres_linux_amd64.tar.gz")
LINUX_ARM64=$(sha "kubectl-podres_linux_arm64.tar.gz")
DARWIN_AMD64=$(sha "kubectl-podres_darwin_amd64.tar.gz")
DARWIN_ARM64=$(sha "kubectl-podres_darwin_arm64.tar.gz")
WINDOWS_AMD64=$(sha "kubectl-podres_windows_amd64.zip")

OLD_VERSION=$(awk '/^  version:/{gsub(/"/, ""); print $2; exit}' "$MANIFEST")

echo "Updating $MANIFEST ($OLD_VERSION → $VERSION)..."

awk \
  -v new_ver="$VERSION" \
  -v old_ver="$OLD_VERSION" \
  -v sha_linux_amd64="$LINUX_AMD64" \
  -v sha_linux_arm64="$LINUX_ARM64" \
  -v sha_darwin_amd64="$DARWIN_AMD64" \
  -v sha_darwin_arm64="$DARWIN_ARM64" \
  -v sha_windows_amd64="$WINDOWS_AMD64" \
  '
  /^  version:/ {
    sub(old_ver, new_ver)
    print; next
  }
  /uri:/ {
    sub(old_ver, new_ver)
    if      (/linux_amd64/)   pending = sha_linux_amd64
    else if (/linux_arm64/)   pending = sha_linux_arm64
    else if (/darwin_amd64/)  pending = sha_darwin_amd64
    else if (/darwin_arm64/)  pending = sha_darwin_arm64
    else if (/windows_amd64/) pending = sha_windows_amd64
    print; next
  }
  /sha256:/ && pending != "" {
    sub(/"[^"]*"/, "\"" pending "\"")
    pending = ""
    print; next
  }
  { print }
  ' "$MANIFEST" > "$MANIFEST.tmp" && mv "$MANIFEST.tmp" "$MANIFEST"

echo "Done. Review and commit:"
echo "  git diff $MANIFEST"
echo "  git add $MANIFEST && git commit -m \"chore: update krew manifest for $VERSION\""
