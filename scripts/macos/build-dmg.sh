#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script must run on macOS." >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
APP_NAME="21loader"
BUNDLE_ID="com.21loader.desktop"

DEFAULT_ICON="$ROOT_DIR/icone.png"
ICON_SOURCE="${LOADER21_ICON:-$DEFAULT_ICON}"
VERSION="${LOADER21_VERSION:-$(date +%Y.%m.%d)}"

OUT_DIR="$ROOT_DIR/dist/macos"
BUILD_DIR="$OUT_DIR/build"
STAGE_DIR="$OUT_DIR/stage"
APP_DIR="$BUILD_DIR/${APP_NAME}.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"
PAYLOAD_DIR="$RESOURCES_DIR/app"
VOL_NAME="${APP_NAME} Installer"

SERVER_BIN_NAME="21loader-server"
LAUNCHER_NAME="$APP_NAME"

usage() {
  cat <<EOF
Usage: $(basename "$0") [--version <version>] [--icon <icon-path>]

Builds a macOS .app bundle and a .dmg installer.

Environment overrides:
  LOADER21_VERSION   Version label used in the dmg filename (default: YYYY.MM.DD)
  LOADER21_ICON      PNG or ICNS file used for the app icon
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --icon)
      ICON_SOURCE="${2:-}"
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

if [[ -z "$VERSION" ]]; then
  echo "Version cannot be empty." >&2
  exit 1
fi

if [[ ! -f "$ICON_SOURCE" ]]; then
  echo "Icon not found: $ICON_SOURCE" >&2
  echo "Provide one with --icon /path/to/icon.png|icns" >&2
  exit 1
fi

DMG_PATH="$OUT_DIR/${APP_NAME}-${VERSION}.dmg"

mkdir -p "$OUT_DIR"
rm -rf "$BUILD_DIR" "$STAGE_DIR"
mkdir -p "$MACOS_DIR" "$PAYLOAD_DIR" "$STAGE_DIR"
cd "$ROOT_DIR"

echo "Building universal macOS binary..."
TMP_BIN_DIR="$BUILD_DIR/tmp-bin"
mkdir -p "$TMP_BIN_DIR"

GO111MODULE=on CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
  go build -trimpath -ldflags="-s -w -X main.appVersion=$VERSION" -o "$TMP_BIN_DIR/${SERVER_BIN_NAME}-arm64" ./cmd/server
GO111MODULE=on CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w -X main.appVersion=$VERSION" -o "$TMP_BIN_DIR/${SERVER_BIN_NAME}-amd64" ./cmd/server

lipo -create \
  -output "$MACOS_DIR/$SERVER_BIN_NAME" \
  "$TMP_BIN_DIR/${SERVER_BIN_NAME}-arm64" \
  "$TMP_BIN_DIR/${SERVER_BIN_NAME}-amd64"
chmod +x "$MACOS_DIR/$SERVER_BIN_NAME"

echo "Copying runtime assets..."
cp -R "$ROOT_DIR/web" "$PAYLOAD_DIR/"
cp -R "$ROOT_DIR/assets" "$PAYLOAD_DIR/"
cp "$ROOT_DIR/icone.png" "$PAYLOAD_DIR/icone.png"

cat > "$MACOS_DIR/$LAUNCHER_NAME" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

SOURCE="${BASH_SOURCE[0]:-$0}"
while [[ -L "$SOURCE" ]]; do
  SOURCE_DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"
  LINK_TARGET="$(readlink "$SOURCE")"
  if [[ "$LINK_TARGET" == /* ]]; then
    SOURCE="$LINK_TARGET"
  else
    SOURCE="$SOURCE_DIR/$LINK_TARGET"
  fi
done
SCRIPT_DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"
APP_PAYLOAD_DIR="$(cd "$SCRIPT_DIR/../Resources/app" && pwd)"
SERVER_BIN="$SCRIPT_DIR/21loader-server"

if [[ "${1:-}" == "update" || "${1:-}" == "version" || "${1:-}" == "--version" ]]; then
  cd "$APP_PAYLOAD_DIR"
  exec "$SERVER_BIN" "$@"
fi

HOST="${LOADER21_HOST:-127.0.0.1}"
PORT="${LOADER21_PORT:-8080}"
LOG_DIR="$HOME/Library/Logs/21loader"
LOG_FILE="$LOG_DIR/server.log"
USER_CLI_DIR="$HOME/.local/bin"
USER_CLI="$USER_CLI_DIR/21loader"

mkdir -p "$LOG_DIR"
mkdir -p "$USER_CLI_DIR"
ln -sf "$SCRIPT_DIR/21loader" "$USER_CLI" 2>/dev/null || true
cd "$APP_PAYLOAD_DIR"

"$SERVER_BIN" --host "$HOST" --port "$PORT" >>"$LOG_FILE" 2>&1 &
SERVER_PID=$!

cleanup() {
  if kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

READY=0
for _ in $(seq 1 100); do
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    break
  fi
  if curl -fsS "http://$HOST:$PORT/healthz" >/dev/null 2>&1; then
    READY=1
    break
  fi
  sleep 0.1
done

if [[ "$READY" -eq 0 ]]; then
  osascript -e 'display alert "21loader" message "Impossible de demarrer le serveur local. Verifie les logs dans ~/Library/Logs/21loader/server.log." as critical'
  wait "$SERVER_PID" || true
  exit 1
fi

open "http://$HOST:$PORT"
wait "$SERVER_PID"
EOF
chmod +x "$MACOS_DIR/$LAUNCHER_NAME"

cat > "$CONTENTS_DIR/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key>
  <string>$APP_NAME</string>
  <key>CFBundleDisplayName</key>
  <string>$APP_NAME</string>
  <key>CFBundleIdentifier</key>
  <string>$BUNDLE_ID</string>
  <key>CFBundleVersion</key>
  <string>$VERSION</string>
  <key>CFBundleShortVersionString</key>
  <string>$VERSION</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleExecutable</key>
  <string>$LAUNCHER_NAME</string>
  <key>CFBundleIconFile</key>
  <string>AppIcon</string>
  <key>LSMinimumSystemVersion</key>
  <string>11.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF

echo "Generating app icon..."
ICON_EXT="${ICON_SOURCE##*.}"
ICON_EXT_LOWER="$(echo "$ICON_EXT" | tr '[:upper:]' '[:lower:]')"

if [[ "$ICON_EXT_LOWER" == "icns" ]]; then
  cp "$ICON_SOURCE" "$RESOURCES_DIR/AppIcon.icns"
else
  if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 is required to convert PNG icon to ICNS." >&2
    echo "Either install python3 + Pillow, or pass an .icns file with --icon." >&2
    exit 1
  fi

  if ! python3 - "$ICON_SOURCE" "$RESOURCES_DIR/AppIcon.icns" <<'PY'
import sys

try:
    from PIL import Image, ImageOps
except Exception as exc:  # pragma: no cover
    print(
        "Pillow is required for PNG->ICNS conversion. "
        "Install it with: python3 -m pip install --user pillow",
        file=sys.stderr,
    )
    raise SystemExit(1) from exc

source = sys.argv[1]
destination = sys.argv[2]

image = Image.open(source).convert("RGBA")
resampling = getattr(Image, "Resampling", Image).LANCZOS
image = ImageOps.fit(image, (1024, 1024), method=resampling, centering=(0.5, 0.5))
image.save(
    destination,
    format="ICNS",
    sizes=[(16, 16), (32, 32), (64, 64), (128, 128), (256, 256), (512, 512), (1024, 1024)],
)
PY
  then
    echo "Icon conversion failed for: $ICON_SOURCE" >&2
    exit 1
  fi
fi

echo "Preparing dmg layout..."
cp -R "$APP_DIR" "$STAGE_DIR/"
ln -s /Applications "$STAGE_DIR/Applications"
cat > "$STAGE_DIR/Install Terminal Command.command" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

APP_LAUNCHER="/Applications/21loader.app/Contents/MacOS/21loader"
TARGET="/usr/local/bin/21loader"

if [[ ! -x "$APP_LAUNCHER" ]]; then
  echo "21loader.app introuvable dans /Applications. Glisse d'abord 21loader.app dans Applications."
  read -r -p "Appuie sur Entree pour fermer..." _
  exit 1
fi

if [[ -w "$(dirname "$TARGET")" ]]; then
  ln -sf "$APP_LAUNCHER" "$TARGET"
else
  sudo ln -sf "$APP_LAUNCHER" "$TARGET"
fi

echo "Commande installee: 21loader"
echo "Tu peux lancer l'app avec: 21loader"
echo "Tu peux mettre a jour avec: 21loader update"
read -r -p "Appuie sur Entree pour fermer..." _
EOF
chmod +x "$STAGE_DIR/Install Terminal Command.command"

rm -f "$DMG_PATH"
echo "Creating dmg..."
hdiutil create \
  -volname "$VOL_NAME" \
  -srcfolder "$STAGE_DIR" \
  -ov \
  -format UDZO \
  "$DMG_PATH" >/dev/null

echo ""
echo "Done."
echo "App bundle: $APP_DIR"
echo "DMG:        $DMG_PATH"
