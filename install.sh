#!/usr/bin/env sh
# install.sh — install the adg CLI from prebuilt GitHub Releases (no Go toolchain).
#
#   curl -fsSL https://raw.githubusercontent.com/d3i-infra/adg/main/install.sh | sh
#
# Installs the latest release into ~/.local/bin. Override the location with
# ADG_INSTALL_DIR or PREFIX; pin a version with ADG_VERSION=v1.1.0. POSIX sh; needs curl.
set -eu

REPO="${ADG_REPO:-d3i-infra/adg}"
INSTALL_DIR="${ADG_INSTALL_DIR:-${PREFIX:+$PREFIX/bin}}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

die() { echo "install.sh: $*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }
have curl || die "curl is required"

# map platform to the goreleaser asset name
os=$(uname -s); arch=$(uname -m)
case "$os" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    MINGW*|MSYS*|CYGWIN*) os=windows ;;
    *) die "unsupported OS: $os" ;;
esac
case "$arch" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) die "unsupported architecture: $arch" ;;
esac
ext=""; [ "$os" = windows ] && ext=".exe"
asset="adg_${os}_${arch}${ext}"

# resolve the download base + version
version="${ADG_VERSION:-}"
base="${ADG_BASE_URL:-}"
if [ -z "$base" ]; then
    if [ -z "$version" ]; then
        # follow the /releases/latest redirect to the tag — no API token or jq
        eff=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
              "https://github.com/$REPO/releases/latest") || die "could not resolve the latest release"
        version=${eff##*/}
        case "$version" in v*) ;; *) die "no published release found (set ADG_VERSION=vX.Y.Z)";; esac
    fi
    base="https://github.com/$REPO/releases/download/$version"
fi
version="${version:-snapshot}"

tmp=$(mktemp); sums="$tmp.sums"
trap 'rm -f "$tmp" "$sums"' EXIT
echo "Downloading adg $version ($asset)…" >&2
curl -fsSL "$base/$asset" -o "$tmp" || die "download failed: $base/$asset"

# verify sha256 when checksums.txt is reachable and lists the asset
if curl -fsSL "$base/checksums.txt" -o "$sums" 2>/dev/null; then
    want=$(awk -v a="$asset" '$2 == a {print $1}' "$sums")
    if [ -n "$want" ]; then
        if have sha256sum; then got=$(sha256sum "$tmp" | awk '{print $1}')
        elif have shasum; then got=$(shasum -a 256 "$tmp" | awk '{print $1}')
        else got=""; fi
        [ -z "$got" ] || [ "$got" = "$want" ] || die "checksum mismatch for $asset"
    fi
fi

mkdir -p "$INSTALL_DIR"
chmod +x "$tmp"
mv -f "$tmp" "$INSTALL_DIR/adg${ext}"
echo "Installed adg $version -> $INSTALL_DIR/adg${ext}" >&2
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) echo "Note: $INSTALL_DIR is not on your PATH; add it to run 'adg'." >&2 ;;
esac
