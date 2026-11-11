#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Makes the AppImage from a folder scripts/build-release.sh built: one file that runs on most
# Linux desktops and servers with nothing installed. Runs on Linux.
#
#   packaging/linux/appimage/build-appimage.sh <version> <package folder> <arch> [out folder]
#
#   arch: amd64 or arm64
#
# appimagetool and the AppImage runtime are downloaded into .tools/appimage, exactly the files
# tools.lock names. APPIMAGETOOL=<program> uses one of your own instead of the download.
#
# The image holds the app, the server, FFmpeg and the licences, as packaging/linux/launcher.sh
# starts them. It is read-only: the library, accounts and settings stay in the account's own
# data folder ($XDG_DATA_HOME/wittymediaserver), and the server never replaces its own files.
set -euo pipefail
cd "$(dirname "$0")/../../.."

VERSION="${1:?version}"; STAGE_DIR="${2:?package folder}"; ARCH="${3:?arch}"; OUT="${4:-dist}"
APP_ID=com.wittywilla.WittyMediaServer
LOCK=packaging/linux/appimage/tools.lock
TOOLS=.tools/appimage

[ "$(uname -s)" = Linux ] || { echo "This needs Linux: appimagetool is a Linux program." >&2; exit 1; }
[ -x "$STAGE_DIR/bin/wittymediaserver" ] || { echo "$STAGE_DIR has no server in bin." >&2; exit 1; }
case "$ARCH" in
  amd64) IMAGE_ARCH=x86_64 ;;
  arm64) IMAGE_ARCH=aarch64 ;;
  *) echo "unknown arch $ARCH: there is an AppImage for amd64 and arm64" >&2; exit 2 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) HOST_ARCH=x86_64 ;;
  aarch64|arm64) HOST_ARCH=aarch64 ;;
  *) echo "appimagetool isn't downloaded for $(uname -m): set APPIMAGETOOL" >&2; HOST_ARCH= ;;
esac
mkdir -p "$OUT" "$TOOLS"

# fetch_locked <out> <name>: exactly the file tools.lock names, whatever is already there.
fetch_locked() {
  local out="$1" name="$2" want url got=""
  # || true: with no line for it, read finds nothing, and the message below is what says so.
  read -r want url < <(awk -v n="$name" '$1 == n { print $2, $3 }' "$LOCK") || true
  if [ -z "${want:-}" ] || [ -z "${url:-}" ]; then
    echo "$LOCK names no file for $name" >&2
    return 1
  fi
  if [ -s "$out" ]; then got=$(sha256sum "$out" | cut -d' ' -f1); fi
  [ "$got" = "$want" ] && return 0
  rm -f "$out"
  curl -fsSL --retry 3 -A wms-build -o "$out.part" "$url" || { rm -f "$out.part"; echo "download failed: $url" >&2; return 1; }
  got=$(sha256sum "$out.part" | cut -d' ' -f1)
  if [ "$got" != "$want" ]; then
    rm -f "$out.part"
    echo "$url does not match $LOCK" >&2
    return 1
  fi
  mv "$out.part" "$out"
}

TOOL="${APPIMAGETOOL:-}"
if [ -z "$TOOL" ]; then
  [ -n "$HOST_ARCH" ] || exit 1
  TOOL="$TOOLS/appimagetool-$HOST_ARCH.AppImage"
  fetch_locked "$TOOL" "appimagetool-$HOST_ARCH"
  chmod +x "$TOOL"
fi
RUNTIME="$TOOLS/runtime-$IMAGE_ARCH"
fetch_locked "$RUNTIME" "runtime-$IMAGE_ARCH"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
app="$work/WittyMediaServer.AppDir"
mkdir -p "$app"
cp -R "$STAGE_DIR/." "$app/"
# Only for archives and for updating from older copies. And no settings.ini beside the program:
# nobody can change a file inside the image, so Settings, Network saves in the data folder.
rm -f "$app/wms" "$app/install.sh" "$app/settings.ini"
# Which kind of copy this is: one the server must not try to replace.
printf '{"product":"WittyMediaServer","kind":"appimage"}\n' > "$app/package-info.json"

install -m 0755 packaging/linux/launcher.sh "$app/AppRun"
# The menu entry, its icon and the description software centres read, where AppImage tools look.
mkdir -p "$app/usr/share/applications" "$app/usr/share/icons/hicolor/256x256/apps" "$app/usr/share/metainfo"
sed -e 's|^Exec=.*|Exec=AppRun|' -e "s|^Icon=.*|Icon=$APP_ID|" "$STAGE_DIR/share/wittymediaserver.desktop" > "$app/$APP_ID.desktop"
grep -q '^Exec=AppRun$' "$app/$APP_ID.desktop" || { echo "$STAGE_DIR/share/wittymediaserver.desktop has no Exec line" >&2; exit 1; }
cp "$app/$APP_ID.desktop" "$app/usr/share/applications/$APP_ID.desktop"
cp "$STAGE_DIR/share/wittymediaserver.png" "$app/$APP_ID.png"
cp "$STAGE_DIR/share/wittymediaserver.png" "$app/usr/share/icons/hicolor/256x256/apps/$APP_ID.png"
ln -s "$APP_ID.png" "$app/.DirIcon"
packaging/linux/flatpak/metainfo.sh "$VERSION" > "$app/usr/share/metainfo/$APP_ID.appdata.xml"
rm -rf "$app/share"

target="$OUT/wittymediaserver-$VERSION-linux-$ARCH.AppImage"
rm -f "$target"
# ARCH names the processor the image is for: appimagetool can't tell from a folder that also
# holds a shell script. Unpacking itself to run means the build machine needs no FUSE.
# --no-appstream: the description is checked when the Flatpak is built, by the tool made for it.
ARCH="$IMAGE_ARCH" VERSION="$VERSION" APPIMAGE_EXTRACT_AND_RUN=1 \
  "$TOOL" --no-appstream --runtime-file "$RUNTIME" "$app" "$target"
chmod +x "$target"
echo "   -> $target"
