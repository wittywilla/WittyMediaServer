#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Makes the Flatpak bundle (.flatpak) from a folder scripts/build-release.sh built. Runs on Linux.
#
#   packaging/linux/flatpak/build-flatpak.sh <version> <package folder> <arch> [out folder]
#
#   arch: amd64 or arm64
#
# Needs flatpak and flatpak-builder. The runtime and SDK the manifest names come from Flathub,
# into this account's own Flatpak folder, the first time. For the other processor's bundle the
# build commands run under QEMU: install qemu-user-static first.
#
# People install it with: flatpak install wittymediaserver-<version>-linux-<arch>.flatpak
set -euo pipefail
cd "$(dirname "$0")/../../.."

VERSION="${1:?version}"; STAGE_DIR="${2:?package folder}"; ARCH="${3:?arch}"; OUT="${4:-dist}"
APP_ID=com.wittywilla.WittyMediaServer
BRANCH=stable
FLATHUB=https://dl.flathub.org/repo/flathub.flatpakrepo
FLATPAK_BUILDER="${FLATPAK_BUILDER:-flatpak-builder}"

command -v flatpak >/dev/null || { echo "flatpak is not installed" >&2; exit 1; }
command -v "$FLATPAK_BUILDER" >/dev/null || { echo "flatpak-builder is not installed" >&2; exit 1; }
[ -x "$STAGE_DIR/bin/wittymediaserver" ] || { echo "$STAGE_DIR has no server in bin." >&2; exit 1; }
[ -f "$STAGE_DIR/WittyMediaServer" ] || { echo "$STAGE_DIR has no app (WittyMediaServer)." >&2; exit 1; }
case "$ARCH" in
  amd64) FLATPAK_ARCH=x86_64 ;;
  arm64) FLATPAK_ARCH=aarch64 ;;
  *) echo "unknown arch $ARCH: there is a Flatpak for amd64 and arm64" >&2; exit 2 ;;
esac
mkdir -p "$OUT"

work="$(mktemp -d)"
# flatpak-builder leaves folders nobody may write in.
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT
stage="$work/stage"
mkdir -p "$stage"
cp -R "$STAGE_DIR/." "$stage/"
# Only for archives and for updating from older copies. And no settings.ini beside the program:
# /app is read-only, so Settings, Network saves in the data folder.
rm -f "$stage/wms" "$stage/install.sh" "$stage/settings.ini"
# Which kind of copy this is: one Flatpak looks after, and the server must not try to replace.
printf '{"product":"WittyMediaServer","kind":"flatpak"}\n' > "$stage/package-info.json"
cp packaging/linux/launcher.sh "$stage/launcher.sh"
sed -e 's|^Exec=.*|Exec=wittymediaserver|' -e "s|^Icon=.*|Icon=$APP_ID|" "$STAGE_DIR/share/wittymediaserver.desktop" > "$stage/$APP_ID.desktop"
grep -q '^Exec=wittymediaserver$' "$stage/$APP_ID.desktop" || { echo "$STAGE_DIR/share/wittymediaserver.desktop has no Exec line" >&2; exit 1; }
packaging/linux/flatpak/metainfo.sh "$VERSION" > "$stage/$APP_ID.metainfo.xml"
cp "packaging/linux/flatpak/$APP_ID.yaml" "$work/$APP_ID.yaml"

flatpak remote-add --user --if-not-exists flathub "$FLATHUB"
# --disable-rofiles-fuse: build machines often have no FUSE, and nothing here needs it.
"$FLATPAK_BUILDER" --user --arch="$FLATPAK_ARCH" --install-deps-from=flathub --force-clean --disable-rofiles-fuse \
  --default-branch="$BRANCH" --state-dir="$work/state" --repo="$work/repo" "$work/build" "$work/$APP_ID.yaml"

target="$OUT/wittymediaserver-$VERSION-linux-$ARCH.flatpak"
rm -f "$target"
# --runtime-repo: where the bundle tells Flatpak to find the runtime when it's installed.
flatpak build-bundle --arch="$FLATPAK_ARCH" --runtime-repo="$FLATHUB" "$work/repo" "$target" "$APP_ID" "$BRANCH"
echo "   -> $target"
