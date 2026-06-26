#!/bin/sh
# siphon installer — downloads a released binary, verifies its SHA-256 against
# the release checksums file, and installs it.
#
#   curl -fsSL https://raw.githubusercontent.com/nixrajput/siphon/main/scripts/install.sh | sh
#
# Environment overrides:
#   SIPHON_VERSION       install a specific tag (e.g. v1.0.0); default: latest
#   SIPHON_INSTALL_DIR   install location; default: /usr/local/bin
#
# POSIX sh, no bashisms. Verifies integrity before extracting; refuses to
# install on a checksum mismatch.
set -eu

REPO="nixrajput/siphon"
INSTALL_DIR="${SIPHON_INSTALL_DIR:-/usr/local/bin}"

err() { printf 'install: %s\n' "$1" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || err "required command not found: $1"; }

# --- prerequisites --------------------------------------------------------
need uname
need tar
# A downloader: prefer curl, fall back to wget.
if command -v curl >/dev/null 2>&1; then
  DL="curl -fsSL"
  DL_O="curl -fsSL -o"
elif command -v wget >/dev/null 2>&1; then
  DL="wget -qO-"
  DL_O="wget -qO"
else
  err "need curl or wget"
fi
# A SHA-256 tool: sha256sum (Linux) or shasum (macOS).
if command -v sha256sum >/dev/null 2>&1; then
  SHA="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  SHA="shasum -a 256"
else
  err "need sha256sum or shasum"
fi

# --- detect OS / arch (match GoReleaser's naming) -------------------------
os=$(uname -s)
case "$os" in
  Linux)  OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)      err "unsupported OS: $os (use the Windows build or build from source)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) err "unsupported architecture: $arch" ;;
esac

# --- resolve version ------------------------------------------------------
VERSION="${SIPHON_VERSION:-}"
if [ -z "$VERSION" ]; then
  # Resolve the latest release tag from the GitHub API.
  VERSION=$($DL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name":' | head -1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  [ -n "$VERSION" ] || err "could not determine the latest release tag"
fi
# GoReleaser archive names use the version WITHOUT the leading "v".
NUM_VERSION=$(printf '%s' "$VERSION" | sed 's/^v//')

ARCHIVE="siphon_${NUM_VERSION}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/${REPO}/releases/download/${VERSION}"

# --- download + verify in a temp dir --------------------------------------
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

printf 'install: downloading %s (%s)\n' "$ARCHIVE" "$VERSION" >&2
$DL_O "$TMP/$ARCHIVE" "$BASE/$ARCHIVE" || err "download failed: $BASE/$ARCHIVE"
$DL_O "$TMP/checksums.txt" "$BASE/checksums.txt" || err "could not download checksums.txt"

# Authenticate checksums.txt itself when cosign is available. Without this, an
# attacker who swaps release assets could replace the archive AND its checksums
# together and go undetected. cosign verify-blob (keyless) confirms checksums.txt
# was signed by this repo's release workflow at THIS tag.
#
# When cosign IS present we fail closed: the release pipeline always produces the
# signature assets, so a missing/invalid signature means a tampered or malformed
# release, not a benign older one. Only when cosign is absent do we fall back to
# checksum-only integrity (requiring cosign would break the common install).
if command -v cosign >/dev/null 2>&1; then
  $DL_O "$TMP/checksums.txt.sig" "$BASE/checksums.txt.sig" 2>/dev/null \
    || err "release is missing checksums.txt.sig — refusing to install (cosign is present, so a signature is expected)"
  $DL_O "$TMP/checksums.txt.pem" "$BASE/checksums.txt.pem" 2>/dev/null \
    || err "release is missing checksums.txt.pem — refusing to install"
  # Pin the exact signer identity: this repo's release.yml at the tag being
  # installed, not just any workflow/any tag.
  identity="https://github.com/${REPO}/.github/workflows/release.yml@refs/tags/${VERSION}"
  cosign verify-blob \
    --certificate "$TMP/checksums.txt.pem" \
    --signature "$TMP/checksums.txt.sig" \
    --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
    --certificate-identity "$identity" \
    "$TMP/checksums.txt" >/dev/null 2>&1 \
    || err "cosign signature verification failed for checksums.txt — refusing to install"
else
  printf 'install: cosign not found; using checksum-only verification (install cosign to verify provenance)\n' >&2
fi

# Verify the archive's SHA-256 against the line for it in checksums.txt.
expected=$(grep " ${ARCHIVE}\$" "$TMP/checksums.txt" | awk '{print $1}')
[ -n "$expected" ] || err "no checksum entry for $ARCHIVE"
actual=$(cd "$TMP" && $SHA "$ARCHIVE" | awk '{print $1}')
if [ "$expected" != "$actual" ]; then
  err "checksum mismatch for $ARCHIVE (expected $expected, got $actual) — refusing to install"
fi

# --- extract + install ----------------------------------------------------
tar -xzf "$TMP/$ARCHIVE" -C "$TMP" siphon || err "could not extract siphon from $ARCHIVE"

if [ -w "$INSTALL_DIR" ] 2>/dev/null || mkdir -p "$INSTALL_DIR" 2>/dev/null; then
  install -m 0755 "$TMP/siphon" "$INSTALL_DIR/siphon" 2>/dev/null \
    || { mv "$TMP/siphon" "$INSTALL_DIR/siphon" && chmod 0755 "$INSTALL_DIR/siphon"; }
else
  # Need elevated permissions for a system dir like /usr/local/bin.
  printf 'install: %s is not writable; retrying with sudo\n' "$INSTALL_DIR" >&2
  need sudo
  sudo install -m 0755 "$TMP/siphon" "$INSTALL_DIR/siphon" \
    || err "could not install to $INSTALL_DIR"
fi

printf 'install: siphon %s installed to %s/siphon\n' "$VERSION" "$INSTALL_DIR" >&2
"$INSTALL_DIR/siphon" --version >&2 2>/dev/null || true
