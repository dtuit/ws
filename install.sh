#!/bin/sh
set -eu

REPO="dtuit/ws"
INSTALL_DIR="${WS_INSTALL_DIR:-}"
VERSION="${WS_VERSION:-}"

usage() {
  cat <<'EOF'
ws installer

Usage:
  curl -LsSf https://raw.githubusercontent.com/dtuit/ws/main/install.sh | sh

Options:
  --version <tag>   Install a specific release tag (for example: v0.1.0)
  --dir <path>      Install into a specific directory
  -h, --help        Show this help

Environment:
  WS_VERSION        Release tag to install
  WS_INSTALL_DIR    Destination directory
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      if [ "$#" -lt 2 ]; then
        echo "error: --version requires a value" >&2
        exit 1
      fi
      VERSION="$2"
      shift 2
      ;;
    --dir)
      if [ "$#" -lt 2 ]; then
        echo "error: --dir requires a value" >&2
        exit 1
      fi
      INSTALL_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required command not found: $1" >&2
    exit 1
  fi
}

download() {
  url="$1"
  dest="$2"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
    return
  fi

  if command -v wget >/dev/null 2>&1; then
    wget -qO "$dest" "$url"
    return
  fi

  echo "error: need curl or wget to download releases" >&2
  exit 1
}

compute_sha256() {
  file="$1"

  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
    return
  fi

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
    return
  fi

  if command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$file" | awk '{print $NF}'
    return
  fi

  echo "error: need sha256sum, shasum, or openssl to verify downloads" >&2
  exit 1
}

detect_os() {
  case "$(uname -s)" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    *)
      echo "error: unsupported operating system: $(uname -s)" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "error: unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

latest_version() {
  tmp="$1"
  download "https://api.github.com/repos/${REPO}/releases/latest" "$tmp"
  sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp" | head -n 1
}

if [ -z "$INSTALL_DIR" ]; then
  if [ "$(id -u)" -eq 0 ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="${HOME}/.local/bin"
  fi
fi

need_cmd tar
need_cmd install
need_cmd mktemp

os="$(detect_os)"
arch="$(detect_arch)"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT INT TERM

if [ -z "$VERSION" ]; then
  VERSION="$(latest_version "$tmpdir/latest.json")"
fi

if [ -z "$VERSION" ]; then
  echo "error: could not determine latest release version" >&2
  exit 1
fi

archive="ws_${VERSION#v}_${os}_${arch}.tar.gz"
archive_url="https://github.com/${REPO}/releases/download/${VERSION}/${archive}"
checksums_url="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

echo "installing ws ${VERSION} for ${os}/${arch}" >&2

download "$archive_url" "$tmpdir/$archive"
download "$checksums_url" "$tmpdir/checksums.txt"

expected="$(awk -v file="$archive" '$2 == file {print $1}' "$tmpdir/checksums.txt")"
if [ -z "$expected" ]; then
  echo "error: checksum for ${archive} not found" >&2
  exit 1
fi

actual="$(compute_sha256 "$tmpdir/$archive")"
if [ "$expected" != "$actual" ]; then
  echo "error: checksum verification failed for ${archive}" >&2
  exit 1
fi

mkdir -p "$tmpdir/extract"
tar -xzf "$tmpdir/$archive" -C "$tmpdir/extract"

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmpdir/extract/ws" "$INSTALL_DIR/ws"

echo "installed ${INSTALL_DIR}/ws" >&2
echo "run: ${INSTALL_DIR}/ws version" >&2
