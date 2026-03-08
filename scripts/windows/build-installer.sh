#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
APP_NAME="21loader"
VERSION="${LOADER21_VERSION:-$(date +%Y.%m.%d)}"
ARCH="amd64"
GOCACHE_DIR="${LOADER21_GOCACHE:-$ROOT_DIR/.gocache}"

usage() {
  cat <<EOF
Usage: $(basename "$0") [--version <version>] [--arch <amd64|arm64>]

Builds a Windows setup executable (.exe) that installs 21loader into:
  %LOCALAPPDATA%\\Programs\\21loader

Environment overrides:
  LOADER21_VERSION   Version label used in output folder names
  LOADER21_GOCACHE   Go build cache directory (default: <repo>/.gocache)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --arch)
      ARCH="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ "$ARCH" != "amd64" && "$ARCH" != "arm64" ]]; then
  echo "Unsupported architecture: $ARCH (use amd64 or arm64)" >&2
  exit 1
fi

if [[ -z "$VERSION" ]]; then
  echo "Version cannot be empty." >&2
  exit 1
fi

OUT_DIR="$ROOT_DIR/dist/windows"
PKG_DIR="$OUT_DIR/${APP_NAME}-${VERSION}-${ARCH}"
SETUP_BUILD_DIR="$OUT_DIR/build/setup-${ARCH}"
SETUP_EXE="$OUT_DIR/${APP_NAME}-${VERSION}-${ARCH}-setup.exe"

mkdir -p "$GOCACHE_DIR"

"$ROOT_DIR/scripts/windows/build-exe.sh" --version "$VERSION" --arch "$ARCH"

rm -rf "$SETUP_BUILD_DIR"
mkdir -p "$SETUP_BUILD_DIR"

cp "$ROOT_DIR/scripts/windows/installer/main.go" "$SETUP_BUILD_DIR/main.go"
cp "$ROOT_DIR/scripts/windows/installer/go.mod" "$SETUP_BUILD_DIR/go.mod"

if ! command -v zip >/dev/null 2>&1; then
  echo "'zip' command is required to create installer payload." >&2
  exit 1
fi

(
  cd "$PKG_DIR"
  zip -rq "$SETUP_BUILD_DIR/payload.zip" .
)

echo "Building setup executable ($ARCH)..."
(
  cd "$SETUP_BUILD_DIR"
  GOCACHE="$GOCACHE_DIR" GO111MODULE=on CGO_ENABLED=0 GOOS=windows GOARCH="$ARCH" \
    go build -trimpath -ldflags="-s -w -H windowsgui" -o "$SETUP_EXE" .
)

echo "Windows setup created: $SETUP_EXE"
