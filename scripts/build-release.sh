#!/usr/bin/env bash
# Builds self-contained WittyMediaServer release archives for every supported platform:
# the app, the server and its command line, FFmpeg, and on Windows the uninstaller. Nothing
# needs to be installed on the target machine. The real installers are made from these
# folders: packaging/windows, packaging/linux, packaging/macos.
#
# What a package holds, and what everything is called, is in internal/layout.
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
#   SIGN=1                sign the checksum list, so installs can update to this build from
#                         Settings. Only for a real release: see docs/PUBLISHING.md. Test builds
#                         stay unsigned, so a signed copy of a test build can't exist.
#   WMS_SIGNING_KEY=      with SIGN=1, the key (default ~/.wms-signing/release.key)
#   REQUIRE_SIGNING=1     with SIGN=1, fail when there is no signing key
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
env -u GOOS -u GOARCH go build -o "$TOOLS/release-sign$HOSTEXE" ./scripts/sign
# Puts an icon, a description and a version on Windows programs, which is what Task Manager
# and a file's Properties show. A build tool only: nothing of it ships.
if ! [ -x "$TOOLS/go-winres$HOSTEXE" ]; then
  GOWORK=off GOBIN="$(cd "$TOOLS" && pwd)" env -u GOOS -u GOARCH go install github.com/tc-hib/go-winres@v0.3.3
fi

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

# fetch_locked <out> <name>: exactly the file scripts/ffmpeg.lock names, whatever is cached or wherever
# it came from. What goes in a release is what the release signature vouches for.
fetch_locked() {
  local out="$1" name="$2" want url got
  read -r want url < <(awk -v n="$name" '$1 == n { print $2, $3 }' "$(dirname "${BASH_SOURCE[0]}")/ffmpeg.lock")
  if [ -z "$want" ] || [ -z "$url" ]; then
    echo "   !! scripts/ffmpeg.lock names no file for $name" >&2
    return 1
  fi
  got=$( [ -s "$out" ] && { sha256sum "$out" 2>/dev/null || shasum -a 256 "$out"; } | cut -d' ' -f1)
  [ "$got" = "$want" ] && return 0
  rm -rf "$out" "${out%.zip}" "${out%.tar.xz}"
  fetch "$out" "$url" || return 1
  got=$( (sha256sum "$out" 2>/dev/null || shasum -a 256 "$out") | cut -d' ' -f1)
  if [ "$want" != "$got" ]; then
    echo "   !! $url does not match scripts/ffmpeg.lock" >&2
    rm -f "$out"
    return 1
  fi
}

# untar <archive> <dir>: extracts once.
untar() { [ -d "$2" ] || { mkdir -p "$2" && tar -xf "$1" -C "$2"; }; }
unzipd() { [ -d "$2" ] || { mkdir -p "$2" && unzip -qo "$1" -d "$2"; }; }

# Puts ffmpeg + ffprobe into $1/bin for $2/$3. Returns 1 if it could not.
bundle_ffmpeg() {
  local dir="$1" os="$2" arch="$3" bin="$1/bin"
  mkdir -p "$bin"
  case "$os/$arch" in
    windows/amd64|windows/arm64)
      local n=win64; [ "$arch" = arm64 ] && n=winarm64
      fetch_locked "$CACHE/$n.zip" "$n" || return 1
      unzipd "$CACHE/$n.zip" "$CACHE/$n"
      cp "$CACHE"/$n/*/bin/ffmpeg.exe "$CACHE"/$n/*/bin/ffprobe.exe "$bin/" ;;
    linux/amd64|linux/arm64)
      local n=linux64; [ "$arch" = arm64 ] && n=linuxarm64
      fetch_locked "$CACHE/$n.tar.xz" "$n" || return 1
      untar "$CACHE/$n.tar.xz" "$CACHE/$n"
      cp "$CACHE"/$n/*/bin/ffmpeg "$CACHE"/$n/*/bin/ffprobe "$bin/" ;;
    linux/arm|linux/386)
      # BtbN has no 32-bit Linux builds; John Van Sickle's static builds do.
      local n=armhf; [ "$arch" = 386 ] && n=i686
      fetch_locked "$CACHE/linux-$n.tar.xz" "linux-$n" || return 1
      untar "$CACHE/linux-$n.tar.xz" "$CACHE/linux-$n"
      cp "$CACHE"/linux-$n/*/ffmpeg "$CACHE"/linux-$n/*/ffprobe "$bin/" ;;
    darwin/*)
      local f
      for f in ffmpeg ffprobe; do
        fetch_locked "$CACHE/mac-$arch-$f.zip" "mac-$arch-$f" || return 1
        # Only the program, wherever the archive keeps it.
        unzip -qoj "$CACHE/mac-$arch-$f.zip" "$f" -d "$bin"
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

# winres <package folder> <arch> <description> <original file name> <gui|cli>: the Windows
# resources for one program, left in its source folder for the next go build. run_build
# removes them after, so a folder never holds another architecture's.
winres() {
  local dir="$1" arch="$2" desc="$3" name="$4" kind="$5"
  local abs_icon; abs_icon="$(cd "$(dirname "$ICON_ICO")" && pwd)/$(basename "$ICON_ICO")"
  (cd "$dir" && "$OLDPWD/$TOOLS/go-winres$HOSTEXE" simply --arch "$arch" --out rsrc --manifest "$kind" \
    --icon "$abs_icon" --product-name WittyMediaServer --product-version "$VERSION" --file-version "$NUMERIC_VERSION.0" \
    --file-description "$desc" --original-filename "$name" --copyright "Copyright WittyWilla" >/dev/null)
}

# The app people open (WittyMediaServer, WittyMediaServer.exe). Pure Go on Windows, Linux and
# FreeBSD; macOS needs CGO so it is built on a Mac (TRAY_DARWIN_DIR) or natively when this
# script runs on macOS.
build_tray() {
  local out="$1" os="$2" arch="$3"
  case "$os" in
    windows)
      winres cmd/wittymediaserver-tray "$arch" "WittyMediaServer: the app with the tray icon" WittyMediaServer.exe gui
      CGO_ENABLED=0 GOOS=windows GOARCH="$arch" go build -trimpath -ldflags="-s -w -H=windowsgui -X main.version=${VERSION}" -o "$out/WittyMediaServer.exe" ./cmd/wittymediaserver-tray
      rm -f cmd/wittymediaserver-tray/rsrc_windows_*.syso ;;
    darwin)
      if [ -n "${TRAY_DARWIN_DIR:-}" ] && [ -f "$TRAY_DARWIN_DIR/$arch/WittyMediaServer" ]; then
        cp "$TRAY_DARWIN_DIR/$arch/WittyMediaServer" "$out/WittyMediaServer"
      elif [ "$(uname -s)" = Darwin ]; then
        CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$out/WittyMediaServer" ./cmd/wittymediaserver-tray
      else
        echo "   (the macOS app is skipped: build it on a Mac or provide TRAY_DARWIN_DIR; the server still works)" >&2
      fi ;;
    *) CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" GOARM=7 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$out/WittyMediaServer" ./cmd/wittymediaserver-tray ;;
  esac
}

# marker <file> <kind>: what says a folder is an installed package, and how it got there.
marker() {
  printf '{"product":"WittyMediaServer","kind":"%s"}\n' "$2" > "$1"
}

# Puts the macOS files where Apple wants them: the app in MacOS, everything else in Helpers.
make_app_bundle() {
  local out="$1" arch="$2" app="$1/WittyMediaServer.app/Contents"
  [ -f "$out/WittyMediaServer" ] || return 0
  mkdir -p "$app/MacOS" "$app/Helpers" "$app/Resources"
  mv "$out/WittyMediaServer" "$app/MacOS/"
  mv "$out/bin"/* "$app/Helpers/" && rmdir "$out/bin"
  mv "$out/package-info.json" "$app/Resources/"
  # A starter set goes with the app, so whatever copies the app has it too (layout.Modules).
  if [ -d "$out/modules" ]; then mv "$out/modules" "$app/Resources/modules"; fi
  "$TOOLS/genicon$HOSTEXE" "$app/Resources/icon.png" 512
  # A Mac's uninstaller is an app of its own, so it's found the way an app is.
  local un="$out/Uninstall WittyMediaServer.app/Contents"
  mkdir -p "$un/MacOS"
  CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$un/MacOS/Uninstall WittyMediaServer" ./cmd/wittymediaserver-uninstall
  cat > "$un/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Uninstall WittyMediaServer</string>
  <key>CFBundleDisplayName</key><string>Uninstall WittyMediaServer</string>
  <key>CFBundleIdentifier</key><string>com.wittywilla.wittymediaserver.uninstall</string>
  <key>CFBundleVersion</key><string>${NUMERIC_VERSION}</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleExecutable</key><string>Uninstall WittyMediaServer</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
EOF
  cat > "$app/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>WittyMediaServer</string>
  <key>CFBundleDisplayName</key><string>WittyMediaServer</string>
  <key>CFBundleIdentifier</key><string>com.wittywilla.wittymediaserver</string>
  <key>CFBundleVersion</key><string>${NUMERIC_VERSION}</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleExecutable</key><string>WittyMediaServer</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSUIElement</key><true/>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSLocalNetworkUsageDescription</key><string>So the WittyMediaServer apps on your phone, TV and other computers can find this server and play your media.</string>
  <key>NSBonjourServices</key><array><string>_wittymedia._tcp</string></array>
</dict></plist>
EOF
  # Unsigned, macOS's Local Network privacy can't tell the server apart from any other
  # program, and it never gets through. Without a Mac, build-pkg.sh signs it.
  if [ "$(uname -s)" = Darwin ]; then
    packaging/macos/sign-app.sh "$out"
  else
    echo "   (the macOS app isn't signed: that needs a Mac, and packaging/macos/build-pkg.sh does it)" >&2
  fi
}

# Windows wants four numbers.
NUMERIC_VERSION="$(printf '%s' "$VERSION" | sed -E 's/^([0-9]+\.[0-9]+\.[0-9]+).*/\1/')"
case "$NUMERIC_VERSION" in [0-9]*.[0-9]*.[0-9]*) ;; *) NUMERIC_VERSION=0.0.0 ;; esac
ICON_ICO="$CACHE/icon.ico"
"$TOOLS/genicon$HOSTEXE" "$ICON_ICO"

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
  rm -rf "$out.zip" "$out.tar.gz"; mkdir -p "$out/bin" "$out/docs"
  echo "==> $t"
  ext=""; [ "$os" = windows ] && ext=".exe"
  server_flags="-s -w -X main.version=${VERSION}"
  if [ -n "${SCREENSCRAPER_DEV_ID:-}" ] && [ -n "${SCREENSCRAPER_DEV_PASSWORD:-}" ]; then
    server_flags="$server_flags -X github.com/wittywilla/wittymediaserver/internal/gamemeta.DevID=$SCREENSCRAPER_DEV_ID -X github.com/wittywilla/wittymediaserver/internal/gamemeta.DevPassword=$SCREENSCRAPER_DEV_PASSWORD"
  fi
  [ "$os" = windows ] && winres cmd/wittymediaserver "$arch" "WittyMediaServer: the server and its command line" wittymediaserver.exe cli
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" GOARM=7 go build -trimpath -ldflags="$server_flags" -o "$out/bin/wittymediaserver$ext" ./cmd/wittymediaserver
  rm -f cmd/wittymediaserver/rsrc_windows_*.syso
  build_tray "$out" "$os" "$arch"
  marker "$out/package-info.json" archive
  # A starter set of signed modules, for a package that works offline (docs/MODULES.md). It's
  # only ever copied: nothing here builds a module.
  if [ -n "${WMS_STARTER:-}" ]; then
    [ -f "$WMS_STARTER/catalog.json" ] && [ -f "$WMS_STARTER/catalog.json.sig" ] || { echo "error: $WMS_STARTER has no signed catalogue" >&2; exit 1; }
    mkdir -p "$out/modules"
    cp -R "$WMS_STARTER/." "$out/modules/"
    rm -f "$out/modules/test.key" "$out/modules/test-key.pub" "$out/modules/.wittymodule"
  fi
  if [ "${SKIP_FFMPEG:-}" != "1" ]; then
    if ! bundle_ffmpeg "$out" "$os" "$arch"; then
      echo "   !! no FFmpeg bundled for $t (the server will use ffmpeg from PATH on that machine)" >&2
      MISSING+=("$t")
    fi
  fi
  cp README.md LICENSE NOTICE THIRD_PARTY_LICENSES.md CHANGELOG.md "$out/docs/"
  case "$os" in
    windows)
      winres cmd/wittymediaserver-uninstall "$arch" "Uninstall WittyMediaServer" "Uninstall WittyMediaServer.exe" gui
      CGO_ENABLED=0 GOOS=windows GOARCH="$arch" go build -trimpath -ldflags="-s -w -H=windowsgui -X main.version=${VERSION}" -o "$out/Uninstall WittyMediaServer.exe" ./cmd/wittymediaserver-uninstall
      rm -f cmd/wittymediaserver-uninstall/rsrc_windows_*.syso ;;
    darwin)  make_app_bundle "$out" "$arch" ;;
    *)       cp packaging/install.sh "$out/"; mkdir -p "$out/share"; "$TOOLS/genicon$HOSTEXE" "$out/share/wittymediaserver.png" 256
             cp packaging/linux/wittymediaserver.desktop "$out/share/"
             [ "$os" = freebsd ] && cp packaging/freebsd/wittymediaserver.rc "$out/share/" ;;
  esac
  # So a copy of 0.5.2-alpha or older can update to this one from Settings. Installers don't carry it.
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" GOARM=7 go build -trimpath -ldflags="-s -w" -o "$out/wms$ext" ./cmd/wms-forwarder
  archive="$name.tar.gz"; [ "$os" = windows ] && archive="$name.zip"
  "$TOOLS/pack$HOSTEXE" "$out" "$DIST/$archive"
  ARCHIVES+=("$archive")
  echo "   -> $DIST/$archive"
done

SUM="sha256sum"; command -v sha256sum >/dev/null 2>&1 || SUM="shasum -a 256"
SUMS="wittymediaserver-${VERSION}-SHA256SUMS.txt"
(cd "$DIST" && $SUM "${ARCHIVES[@]}" > "$SUMS")
# Sign the checksum list: the in-app updater installs nothing without it.
rm -f "$DIST/$SUMS.sig"
KEY="${WMS_SIGNING_KEY:-$HOME/.wms-signing/release.key}"
if [ "${SIGN:-}" != "1" ]; then
  echo "   (not signed: SIGN=1 signs a real release)"
elif [ -f "$KEY" ]; then
  "$TOOLS/release-sign$HOSTEXE" sign -key "$KEY" "$DIST/$SUMS"
  "$TOOLS/release-sign$HOSTEXE" verify "$DIST/$SUMS" || { echo "error: the signing key is not the one built into the server (internal/update/keys.go)" >&2; exit 1; }
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
