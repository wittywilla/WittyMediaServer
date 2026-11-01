#!/usr/bin/env bash
# Builds self-contained WittyMediaServer release archives for every supported
# platform: the wms server + tray launcher + bundled FFmpeg + installers.
# Nothing needs to be installed on the target machine.
#
# Runs on Linux, macOS and Windows (Git Bash / MSYS2). Needs Go, Node.js,
# curl, unzip and tar (with xz support). No zip tool is needed.
#
# usage: scripts/build-release.sh [version] [target ...]
#
#   default targets:
#     windows/amd64 windows/arm64
#     darwin/amd64 darwin/arm64
#     linux/amd64 linux/arm64 linux/arm (ARMv7, 32-bit Raspberry Pi OS) linux/386
#     freebsd/amd64 freebsd/arm64
#
# env:
#   SKIP_WEB=1            reuse web/dist
#   SKIP_FFMPEG=1         do not bundle FFmpeg (archives then use ffmpeg from PATH)
#   STRICT_FFMPEG=1       fail if any archive could not get its FFmpeg
#   FFMPEG_FREEBSD_DIR=   static FreeBSD ffmpeg/ffprobe: <dir>/<arch>/, <dir>/ffmpeg-freebsd-<arch>/
#                         or (amd64 only) <dir>/ (see scripts/build-ffmpeg-static.sh)
#   TRAY_DARWIN_DIR=      <dir>/<arch>/WittyMediaServer tray binaries built on a Mac (needs CGO)
#   WMS_SIGNING_KEY=      release signing key (default ~/.wms-signing/release.key, see
#                         docs/PUBLISHING.md); signs the checksum list so installs can update
#   REQUIRE_SIGNING=1     fail when there is no signing key
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
VERSION="${VERSION#v}"
shift || true
TARGETS=("$@")
if [ ${#TARGETS[@]} -eq 0 ]; then
  TARGETS=(windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 linux/arm linux/386 freebsd/amd64 freebsd/arm64)
fi
DIST="${DIST:-dist}" # DIST=elsewhere builds into another folder
CACHE=.tools/ffmpeg-dl
TOOLS=.tools/bin
mkdir -p "$DIST" "$CACHE" "$TOOLS"
MISSING=()

if [ "${SKIP_WEB:-}" != "1" ]; then
  echo "==> building web client"
  # Exactly what the lock file says, and no install scripts from packages.
  (cd web && npm ci --ignore-scripts --no-audit --no-fund && npm run build)
fi

# Host tools (archive packer, icon renderer), built once for this machine.
HOSTEXE="$(go env GOEXE)"
echo "==> building host tools"
env -u GOOS -u GOARCH go build -o "$TOOLS/pack$HOSTEXE" ./scripts/pack
env -u GOOS -u GOARCH go build -o "$TOOLS/genicon$HOSTEXE" ./scripts/genicon
env -u GOOS -u GOARCH go build -o "$TOOLS/wms-sign$HOSTEXE" ./cmd/wms-sign

# fetch <out> <url> [url...]: downloads the first URL that works (cached).
fetch() {
  local out="$1"; shift
  [ -s "$out" ] && return 0
  local url i
  for url in "$@"; do
    for i in 1 2 3; do
      if curl -fsSL --retry 2 -A wms-build -o "$out.part" "$url"; then mv "$out.part" "$out"; return 0; fi
      sleep $((i * 2))
    done
    echo "   (download failed: $url)" >&2
  done
  rm -f "$out.part"
  return 1
}

# fetch_btbn <out> <file>: a rolling FFmpeg build, checked against the checksums
# published with it (a download that was cut short or altered on the way fails).
fetch_btbn() {
  local out="$1" file="$2" want got
  [ -s "$out" ] && return 0
  fetch "$out" "$BTBN/$file" || return 1
  rm -f "$CACHE/btbn.sha256"
  fetch "$CACHE/btbn.sha256" "$BTBN/checksums.sha256" || { rm -f "$out"; return 1; }
  want=$(grep -E "[ *]$file\$" "$CACHE/btbn.sha256" | cut -d' ' -f1 | head -1)
  got=$( (sha256sum "$out" 2>/dev/null || shasum -a 256 "$out") | cut -d' ' -f1)
  if [ -z "$want" ] || [ "$want" != "$got" ]; then
    echo "   !! $file does not match its published checksum" >&2
    rm -f "$out"
    return 1
  fi
}

# untar <archive> <dir>: extracts once.
untar() { [ -d "$2" ] || { mkdir -p "$2" && tar -xf "$1" -C "$2"; }; }
unzipd() { [ -d "$2" ] || { mkdir -p "$2" && unzip -qo "$1" -d "$2"; }; }

BTBN=https://github.com/BtbN/FFmpeg-Builds/releases/download/latest
JVS=https://johnvansickle.com/ffmpeg/releases
MR=https://ffmpeg.martin-riedl.de/redirect/latest

# Puts ffmpeg + ffprobe into $1/bin for $2/$3. Returns 1 if it could not.
bundle_ffmpeg() {
  local dir="$1" os="$2" arch="$3" bin="$1/bin"
  mkdir -p "$bin"
  case "$os/$arch" in
    windows/amd64|windows/arm64)
      local n=win64; [ "$arch" = arm64 ] && n=winarm64
      fetch_btbn "$CACHE/$n.zip" "ffmpeg-master-latest-$n-gpl.zip" || return 1
      unzipd "$CACHE/$n.zip" "$CACHE/$n"
      cp "$CACHE"/$n/*/bin/ffmpeg.exe "$CACHE"/$n/*/bin/ffprobe.exe "$bin/" ;;
    linux/amd64|linux/arm64)
      local n=linux64; [ "$arch" = arm64 ] && n=linuxarm64
      fetch_btbn "$CACHE/$n.tar.xz" "ffmpeg-master-latest-$n-gpl.tar.xz" || return 1
      untar "$CACHE/$n.tar.xz" "$CACHE/$n"
      cp "$CACHE"/$n/*/bin/ffmpeg "$CACHE"/$n/*/bin/ffprobe "$bin/" ;;
    linux/arm|linux/386)
      # BtbN has no 32-bit Linux builds; John Van Sickle's static builds do.
      local n=armhf; [ "$arch" = 386 ] && n=i686
      fetch "$CACHE/linux-$n.tar.xz" "$JVS/ffmpeg-release-$n-static.tar.xz" || return 1
      untar "$CACHE/linux-$n.tar.xz" "$CACHE/linux-$n"
      cp "$CACHE"/linux-$n/*/ffmpeg "$CACHE"/linux-$n/*/ffprobe "$bin/" ;;
    darwin/*)
      # Release links are sometimes missing for one architecture; snapshots
      # (current master) are the fallback.
      local f
      for f in ffmpeg ffprobe; do
        fetch "$CACHE/mac-$arch-$f.zip" "$MR/macos/$arch/release/$f.zip" "$MR/macos/$arch/snapshot/$f.zip" || return 1
        unzip -qo "$CACHE/mac-$arch-$f.zip" -d "$bin"
      done ;;
    freebsd/*)
      local d src=""
      for d in "${FFMPEG_FREEBSD_DIR:-}/$arch" "${FFMPEG_FREEBSD_DIR:-}/ffmpeg-freebsd-$arch"; do
        [ -n "${FFMPEG_FREEBSD_DIR:-}" ] && [ -f "$d/ffmpeg" ] && src="$d" && break
      done
      if [ -z "$src" ] && [ "$arch" = amd64 ] && [ -n "${FFMPEG_FREEBSD_DIR:-}" ] && [ -f "$FFMPEG_FREEBSD_DIR/ffmpeg" ]; then src="$FFMPEG_FREEBSD_DIR"; fi
      [ -n "$src" ] || return 1
      cp "$src/ffmpeg" "$src/ffprobe" "$bin/" ;;
    *) return 1 ;;
  esac
}

# Tray launcher (WittyMediaServer / WittyMediaServer.exe). Pure Go on Windows,
# Linux and FreeBSD; macOS needs CGO so it is built on a Mac (TRAY_DARWIN_DIR)
# or natively when this script runs on macOS.
build_tray() {
  local out="$1" os="$2" arch="$3"
  case "$os" in
    windows) CGO_ENABLED=0 GOOS=windows GOARCH="$arch" go build -trimpath -ldflags="-s -w -H=windowsgui -X main.version=${VERSION}" -o "$out/WittyMediaServer.exe" ./cmd/wms-tray ;;
    darwin)
      if [ -n "${TRAY_DARWIN_DIR:-}" ] && [ -f "$TRAY_DARWIN_DIR/$arch/WittyMediaServer" ]; then
        cp "$TRAY_DARWIN_DIR/$arch/WittyMediaServer" "$out/WittyMediaServer"
      elif [ "$(uname -s)" = Darwin ]; then
        CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$out/WittyMediaServer" ./cmd/wms-tray
      else
        echo "   (macOS tray launcher skipped: build it on a Mac or provide TRAY_DARWIN_DIR; wms still works)" >&2
      fi ;;
    *) CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" GOARM=7 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$out/WittyMediaServer" ./cmd/wms-tray ;;
  esac
}

# Wraps the macOS files in a menu-bar .app (LSUIElement) next to the plain binaries.
make_app_bundle() {
  local out="$1" app="$1/WittyMediaServer.app/Contents"
  [ -f "$out/WittyMediaServer" ] || return 0
  mkdir -p "$app/MacOS" "$app/Resources"
  cp "$out/WittyMediaServer" "$out/wms" "$app/MacOS/"
  [ -f "$out/wittyplay-host" ] && cp "$out/wittyplay-host" "$app/MacOS/"
  [ -d "$out/bin" ] && cp -R "$out/bin" "$app/MacOS/"
  "$TOOLS/genicon$HOSTEXE" "$app/Resources/icon.png" 512
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

ARCHIVES=()
for t in "${TARGETS[@]}"; do
  os="${t%/*}"; arch="${t#*/}"
  label="$arch"; [ "$os/$arch" = linux/arm ] && label=armv7
  name="wittymediaserver-${VERSION}-${os}-${label}"
  out="$DIST/$name"
  # A package folder someone runs in place keeps its data (library, accounts,
  # progress) in data/: never delete that.
  if [ -e "$out/data" ]; then
    echo "error: $out/data exists: a copy is being used from this folder, and its data must not be deleted." >&2
    echo "       Stop it and move the data folder out, or build elsewhere: DIST=dist/next $0 $VERSION" >&2
    exit 1
  fi
  # Moved first, which fails while a copy is running from it. Windows lets rm take a
  # running program's files, leaving that copy with nothing to fall back on.
  if [ -d "$out" ]; then
    if ! mv "$out" "$out.being-replaced"; then
      echo "error: $out is in use: a copy is running from it. Nothing was changed." >&2
      echo "       Stop it, or build elsewhere: DIST=dist/next $0 $VERSION" >&2
      exit 1
    fi
    rm -rf "$out.being-replaced"
  fi
  rm -rf "$out.zip" "$out.tar.gz" "$out-setup.exe"; mkdir -p "$out"
  echo "==> $t"
  ext=""; [ "$os" = windows ] && ext=".exe"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" GOARM=7 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$out/wms$ext" ./cmd/wms
  build_tray "$out" "$os" "$arch"
  # The separate process that runs games (docs/GAMING.md). FreeBSD's cgo-free
  # build needs one package compiled with the standard library's privileges.
  host_flags=(); [ "$os" = freebsd ] && host_flags=(-gcflags="github.com/ebitengine/purego/internal/fakecgo=-std")
  abs_out=$(cd "$out" && pwd)
  (cd modules/wittyplay && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" GOARM=7 go build -trimpath ${host_flags[@]+"${host_flags[@]}"} -ldflags="-s -w" -o "$abs_out/wittyplay-host$ext" ./cmd/wittyplay-host)
  if [ "${SKIP_FFMPEG:-}" != "1" ]; then
    if ! bundle_ffmpeg "$out" "$os" "$arch"; then
      echo "   !! no FFmpeg bundled for $t (wms will use ffmpeg from PATH on that machine)" >&2
      rmdir "$out/bin" 2>/dev/null || true
      MISSING+=("$t")
    fi
  fi
  cp README.md LICENSE NOTICE THIRD_PARTY_LICENSES.md CHANGELOG.md "$out/" 2>/dev/null || true
  case "$os" in
    windows) cp scripts/install.ps1 scripts/uninstall.ps1 scripts/start-wms.cmd "$out/"; "$TOOLS/genicon$HOSTEXE" "$out/icon.ico"
             # Per-user installer and uninstaller (also installs an unpacked folder).
             CGO_ENABLED=0 GOOS=windows GOARCH="$arch" go build -trimpath -ldflags="-s -w -H=windowsgui -X main.version=${VERSION}" -o "$out/wms-setup.exe" ./cmd/wms-setup ;;
    darwin)  cp scripts/install.sh "$out/"; make_app_bundle "$out" ;;
    *)       cp scripts/install.sh "$out/"; "$TOOLS/genicon$HOSTEXE" "$out/icon.png" 256
             sed "s|@PREFIX@|/opt/wittymediaserver|g" scripts/wittymediaserver.desktop > "$out/wittymediaserver.desktop" ;;
  esac
  archive="$name.tar.gz"; [ "$os" = windows ] && archive="$name.zip"
  "$TOOLS/pack$HOSTEXE" "$out" "$DIST/$archive"
  ARCHIVES+=("$archive")
  echo "   -> $DIST/$archive"
  if [ "$os" = windows ]; then
    # The setup .exe is the installer with the package appended.
    cat "$out/wms-setup.exe" "$DIST/$archive" > "$DIST/$name-setup.exe"
    ARCHIVES+=("$name-setup.exe")
    echo "   -> $DIST/$name-setup.exe"
  fi
done

SUM="sha256sum"; command -v sha256sum >/dev/null 2>&1 || SUM="shasum -a 256"
SUMS="wittymediaserver-${VERSION}-SHA256SUMS.txt"
(cd "$DIST" && $SUM "${ARCHIVES[@]}" > "$SUMS")
# Sign the checksum list: the in-app updater installs nothing without it.
rm -f "$DIST/$SUMS.sig"
KEY="${WMS_SIGNING_KEY:-$HOME/.wms-signing/release.key}"
if [ -f "$KEY" ]; then
  "$TOOLS/wms-sign$HOSTEXE" sign -key "$KEY" "$DIST/$SUMS"
  "$TOOLS/wms-sign$HOSTEXE" verify "$DIST/$SUMS" || { echo "error: the signing key is not the one built into wms (internal/update/keys.go)" >&2; exit 1; }
else
  echo "   !! no signing key at $KEY: $SUMS is not signed, and installs can't update to this release from Settings" >&2
  [ "${REQUIRE_SIGNING:-}" = "1" ] && exit 1
fi
echo
echo "==> built ${#ARCHIVES[@]} files in $DIST/ (checksums: $SUMS$([ -f "$DIST/$SUMS.sig" ] && echo ", signed"))"
if [ ${#MISSING[@]} -gt 0 ]; then
  echo "   without bundled FFmpeg: ${MISSING[*]}"
  [ "${STRICT_FFMPEG:-}" = "1" ] && exit 1
fi
echo "done."
