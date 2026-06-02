#!/bin/sh
# Install the latest ai-qa-agent-cli release binary.
#
#   curl -fsSL https://raw.githubusercontent.com/bitrise-io/ai-qa-agent-cli/main/install.sh | sh
#
# Environment overrides:
#   VERSION   release tag to install (default: latest, e.g. VERSION=v0.1.0)
#   BIN_DIR   install destination   (default: /usr/local/bin, falls back to
#             ~/.local/bin when /usr/local/bin is not writable and sudo is absent)
set -eu

REPO="bitrise-io/ai-qa-agent-cli"
BINARY="ai-qa-agent-cli"

err() { echo "install: $*" >&2; exit 1; }
info() { echo "install: $*" >&2; }

need() { command -v "$1" >/dev/null 2>&1 || err "required command not found: $1"; }
need curl
need tar
need uname

# --- detect platform ---------------------------------------------------------
os="$(uname -s)"
case "$os" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *) err "unsupported OS: $os (only linux and darwin are published)" ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64)  arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) err "unsupported architecture: $arch" ;;
esac

# --- resolve version ---------------------------------------------------------
version="${VERSION:-}"
if [ -z "$version" ]; then
  # The releases/latest URL redirects to .../tag/<version>; read it from the
  # final effective URL so we need neither the GitHub API nor jq.
  version="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
    "https://github.com/$REPO/releases/latest" | sed 's#.*/tag/##')"
  [ -n "$version" ] || err "could not determine the latest version"
fi

asset="${BINARY}_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$version"
info "installing $BINARY $version ($os/$arch)"

# --- download + verify -------------------------------------------------------
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

curl -fsSL -o "$tmp/$asset" "$base/$asset" \
  || err "download failed: $base/$asset"
curl -fsSL -o "$tmp/SHA256SUMS" "$base/SHA256SUMS" \
  || err "download failed: $base/SHA256SUMS"

info "verifying checksum"
if command -v sha256sum >/dev/null 2>&1; then
  ( cd "$tmp" && sha256sum --ignore-missing -c SHA256SUMS >/dev/null ) \
    || err "checksum verification failed"
elif command -v shasum >/dev/null 2>&1; then
  want="$(grep " $asset\$" "$tmp/SHA256SUMS" | awk '{print $1}')"
  got="$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')"
  [ -n "$want" ] && [ "$want" = "$got" ] || err "checksum verification failed"
else
  err "no sha256sum or shasum available to verify the download"
fi

tar -xzf "$tmp/$asset" -C "$tmp" || err "failed to extract $asset"
[ -f "$tmp/$BINARY" ] || err "archive did not contain $BINARY"
chmod +x "$tmp/$BINARY"

# --- install -----------------------------------------------------------------
dest="${BIN_DIR:-/usr/local/bin}"
if [ -n "${BIN_DIR:-}" ]; then
  # An explicit BIN_DIR is honored as-is (created if needed).
  mkdir -p "$dest" || err "cannot create BIN_DIR: $dest"
fi

if [ -d "$dest" ] && [ -w "$dest" ]; then
  install -m 0755 "$tmp/$BINARY" "$dest/$BINARY"
elif command -v sudo >/dev/null 2>&1 && [ -t 0 ]; then
  info "installing to $dest (requires sudo)"
  sudo install -m 0755 "$tmp/$BINARY" "$dest/$BINARY"
else
  dest="$HOME/.local/bin"
  mkdir -p "$dest"
  install -m 0755 "$tmp/$BINARY" "$dest/$BINARY"
fi

info "installed $dest/$BINARY"

# --- PATH hint ---------------------------------------------------------------
case ":$PATH:" in
  *":$dest:"*) ;;
  *) info "note: $dest is not on your PATH — add it, e.g.:"
     info "      export PATH=\"$dest:\$PATH\"" ;;
esac
