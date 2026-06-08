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

Builds a portable Windows package with:
  - 21loader-server.exe
  - web/ and assets/ runtime folders
  - 21loader.cmd launcher

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
APP_DIR="$PKG_DIR/app"
SERVER_BIN="$APP_DIR/21loader-server.exe"
LAUNCHER_SRC="$ROOT_DIR/scripts/windows/launch-21loader.cmd"
LAUNCHER_DST="$PKG_DIR/${APP_NAME}.cmd"
README_DST="$PKG_DIR/README-WINDOWS.txt"

if [[ ! -f "$LAUNCHER_SRC" ]]; then
  echo "Launcher template not found: $LAUNCHER_SRC" >&2
  exit 1
fi

rm -rf "$PKG_DIR"
mkdir -p "$APP_DIR"
mkdir -p "$GOCACHE_DIR"

echo "Building Windows binary ($ARCH)..."
GOCACHE="$GOCACHE_DIR" GO111MODULE=on CGO_ENABLED=0 GOOS=windows GOARCH="$ARCH" \
  go build -trimpath -ldflags="-s -w -X main.appVersion=$VERSION" -o "$SERVER_BIN" ./cmd/server

echo "Copying runtime assets..."
cp -R "$ROOT_DIR/web" "$APP_DIR/"
cp -R "$ROOT_DIR/assets" "$APP_DIR/"
cp "$ROOT_DIR/icone.png" "$APP_DIR/icone.png"
cp "$LAUNCHER_SRC" "$LAUNCHER_DST"

cat > "$README_DST" <<EOF
21loader - Windows package ($ARCH)
==================================

Quick start:
1. Double-click ${APP_NAME}.cmd, or type ${APP_NAME} in a new terminal after setup installer integration.
2. Browser should open automatically on http://127.0.0.1:8080
3. Update from GitHub Releases with: ${APP_NAME} update

Notes:
- Logs: %APPDATA%\\21loader\\Logs\\21loader\\server.log
- Change host/port before launch (optional):
  set LOADER21_HOST=127.0.0.1
  set LOADER21_PORT=8080
- Keep the app/ folder next to ${APP_NAME}.cmd
EOF

if command -v zip >/dev/null 2>&1; then
  ZIP_PATH="$OUT_DIR/${APP_NAME}-${VERSION}-${ARCH}.zip"
  rm -f "$ZIP_PATH"
  (
    cd "$OUT_DIR"
    zip -rq "$ZIP_PATH" "$(basename "$PKG_DIR")"
  )
  echo "Created zip: $ZIP_PATH"
fi

echo "Windows package created: $PKG_DIR"
