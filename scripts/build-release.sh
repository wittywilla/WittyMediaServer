#!/usr/bin/env bash
# Builds self-contained WittyMediaServer release archives for every platform:
# the wms binary + bundled FFmpeg + installer scripts. Nothing to install on
# the target machine.
#
# usage: scripts/build-release.sh [version] [target ...]
#   targets: windows/amd64 windows/arm64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 freebsd/amd64
# env:
#   SKIP_WEB=1            reuse web/dist
#   SKIP_FFMPEG=1         do not download ffmpeg (archives will need WMS_FFMPEG)
#   FFMPEG_FREEBSD_DIR=   directory with static freebsd ffmpeg/ffprobe (see build-ffmpeg-static.sh)
#   TRAY_DARWIN_DIR=      directory with <arch>/WittyMediaServer tray binaries built on macOS (CGO)
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
shift || true
TARGETS=("$@")
if [ ${#TARGETS[@]} -eq 0 ]; then
  TARGETS=(windows/amd64 windows/arm64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 freebsd/amd64)
fi
DIST=dist
CACHE=.tools/ffmpeg-dl
mkdir -p "$DIST" "$CACHE"

if [ "${SKIP_WEB:-}" != "1" ]; then
  echo "==> building web client"
  (cd web && (npm ci --no-audit --no-fund || npm install --no-audit --no-fund) && npm run build)
fi

fetch() { # url out
  local url="$1" out="$2" i
  [ -s "$out" ] && return 0
  for i in 1 2 3 4; do
    if curl -fL --retry 3 -A wms-build -o "$out.part" "$url"; then mv "$out.part" "$out"; return 0; fi
    sleep $((i * 3))
  done
  echo "download failed: $url" >&2; return 1
}

# Puts ffmpeg + ffprobe into $1/bin for GOOS/GOARCH $2/$3.
bundle_ffmpeg() {
  local dir="$1" os="$2" arch="$3" bin="$dir/bin"
  mkdir -p "$bin"
  case "$os/$arch" in
    windows/amd64) fetch https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip "$CACHE/win64.zip"
      unzip -qo "$CACHE/win64.zip" -d "$CACHE/win64"; cp "$CACHE"/win64/*/bin/ffmpeg.exe "$CACHE"/win64/*/bin/ffprobe.exe "$bin/" ;;
    windows/arm64) fetch https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-winarm64-gpl.zip "$CACHE/winarm64.zip"
      unzip -qo "$CACHE/winarm64.zip" -d "$CACHE/winarm64"; cp "$CACHE"/winarm64/*/bin/ffmpeg.exe "$CACHE"/winarm64/*/bin/ffprobe.exe "$bin/" ;;
    linux/amd64) fetch https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-gpl.tar.xz "$CACHE/linux64.tar.xz"
      mkdir -p "$CACHE/linux64"; tar -xJf "$CACHE/linux64.tar.xz" -C "$CACHE/linux64"; cp "$CACHE"/linux64/*/bin/ffmpeg "$CACHE"/linux64/*/bin/ffprobe "$bin/" ;;
    linux/arm64) fetch https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linuxarm64-gpl.tar.xz "$CACHE/linuxarm64.tar.xz"
      mkdir -p "$CACHE/linuxarm64"; tar -xJf "$CACHE/linuxarm64.tar.xz" -C "$CACHE/linuxarm64"; cp "$CACHE"/linuxarm64/*/bin/ffmpeg "$CACHE"/linuxarm64/*/bin/ffprobe "$bin/" ;;
    darwin/*)
      fetch "https://ffmpeg.martin-riedl.de/redirect/latest/macos/$arch/release/ffmpeg.zip" "$CACHE/mac-$arch-ffmpeg.zip"
      fetch "https://ffmpeg.martin-riedl.de/redirect/latest/macos/$arch/release/ffprobe.zip" "$CACHE/mac-$arch-ffprobe.zip"
      unzip -qo "$CACHE/mac-$arch-ffmpeg.zip" -d "$bin"; unzip -qo "$CACHE/mac-$arch-ffprobe.zip" -d "$bin" ;;
    freebsd/*)
      if [ -n "${FFMPEG_FREEBSD_DIR:-}" ] && [ -x "$FFMPEG_FREEBSD_DIR/ffmpeg" ]; then
        cp "$FFMPEG_FREEBSD_DIR/ffmpeg" "$FFMPEG_FREEBSD_DIR/ffprobe" "$bin/"
      else
        echo "   (no FreeBSD ffmpeg provided; build one with scripts/build-ffmpeg-static.sh and set FFMPEG_FREEBSD_DIR," >&2
        echo "    or install ffmpeg with 'pkg install ffmpeg' on the target - wms finds it on PATH)" >&2
      fi ;;
  esac
  chmod +x "$bin"/* 2>/dev/null || true
}

# Tray launcher (WittyMediaServer / WittyMediaServer.exe). Pure Go on Windows,
# Linux and FreeBSD; macOS needs CGO so it is built on a Mac (TRAY_DARWIN_DIR)
# or natively when this script runs on macOS.
build_tray() {
  local out="$1" os="$2" arch="$3"
  case "$os" in
    windows) CGO_ENABLED=0 GOOS=windows GOARCH="$arch" go build -trimpath -ldflags="-s -w -H=windowsgui -X main.version=${VERSION}" -o "$out/WittyMediaServer.exe" ./cmd/wms-tray ;;
    darwin)
      if [ -n "${TRAY_DARWIN_DIR:-}" ] && [ -x "$TRAY_DARWIN_DIR/$arch/WittyMediaServer" ]; then
        cp "$TRAY_DARWIN_DIR/$arch/WittyMediaServer" "$out/WittyMediaServer"
      elif [ "$(uname -s)" = Darwin ]; then
        CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$out/WittyMediaServer" ./cmd/wms-tray
      else
        echo "   (macOS tray launcher skipped: build it on a Mac or provide TRAY_DARWIN_DIR)" >&2
      fi ;;
    *) CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$out/WittyMediaServer" ./cmd/wms-tray ;;
  esac
}

# Wraps the macOS files in a menu-bar .app (LSUIElement) next to the plain binaries.
make_app_bundle() {
  local out="$1" app="$1/WittyMediaServer.app/Contents"
  [ -x "$out/WittyMediaServer" ] || return 0
  mkdir -p "$app/MacOS" "$app/Resources"
  cp "$out/WittyMediaServer" "$out/wms" "$app/MacOS/"
  [ -d "$out/bin" ] && cp -R "$out/bin" "$app/MacOS/"
  go run ./scripts/genicon "$app/Resources/icon.png" 512
  cat > "$app/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>WittyMediaServer</string>
  <key>CFBundleDisplayName</key><string>WittyMediaServer</string>
  <key>CFBundleIdentifier</key><string>com.wittywilla.wittymediaserver</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleExecutable</key><string>WittyMediaServer</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSUIElement</key><true/>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
EOF
}

for t in "${TARGETS[@]}"; do
  os="${t%/*}"; arch="${t#*/}"
  name="wittymediaserver-${VERSION}-${os}-${arch}"
  out="$DIST/$name"
  rm -rf "$out"; mkdir -p "$out"
  echo "==> $t"
  ext=""; [ "$os" = windows ] && ext=".exe"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$out/wms$ext" ./cmd/wms
  build_tray "$out" "$os" "$arch"
  [ "${SKIP_FFMPEG:-}" = "1" ] || bundle_ffmpeg "$out" "$os" "$arch"
  cp README.md LICENSE NOTICE THIRD_PARTY_LICENSES.md "$out/" 2>/dev/null || true
  case "$os" in
    windows) cp scripts/install.ps1 scripts/uninstall.ps1 "$out/"; cp scripts/start-wms.cmd "$out/"; go run ./scripts/genicon "$out/icon.ico" ;;
    darwin)  cp scripts/install.sh "$out/"; chmod +x "$out/install.sh"; make_app_bundle "$out" ;;
    *)       cp scripts/install.sh "$out/"; chmod +x "$out/install.sh"; go run ./scripts/genicon "$out/icon.png" 256
             sed "s|@PREFIX@|/opt/wittymediaserver|g" scripts/wittymediaserver.desktop > "$out/wittymediaserver.desktop" ;;
  esac
  (cd "$DIST" && if [ "$os" = windows ]; then rm -f "$name.zip"; zip -qr "$name.zip" "$name"; else rm -f "$name.tar.gz"; tar -czf "$name.tar.gz" "$name"; fi)
  echo "   -> $DIST/$name.$([ "$os" = windows ] && echo zip || echo tar.gz)"
done
echo "done."
