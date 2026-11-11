#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Makes the snap from a folder scripts/build-release.sh built. Runs on Linux.
#
#   packaging/linux/snap/build-snap.sh <version> <package folder> <arch> [out folder]
#
#   arch: amd64 or arm64
#
# Needs Snapcraft: sudo snap install snapcraft --classic
# Snapcraft builds in an LXD container of its own. On a machine that is already Ubuntu 24.04 and
# is thrown away after, as CI's are, SNAPCRAFT_BUILD_ENVIRONMENT=host builds on the machine itself.
#
# The snap holds the server, its command line, FFmpeg and the licences. It has no tray app: a
# snap's service is started and stopped with snap, and its data is in
# /var/snap/wittymediaserver/common.
#
# People install it with: sudo snap install --dangerous wittymediaserver-<version>-linux-<arch>.snap
set -euo pipefail
cd "$(dirname "$0")/../../.."

VERSION="${1:?version}"; STAGE_DIR="${2:?package folder}"; ARCH="${3:?arch}"; OUT="${4:-dist}"
SNAPCRAFT="${SNAPCRAFT:-snapcraft}"

command -v "$SNAPCRAFT" >/dev/null || { echo "snapcraft is not installed: see the top of this script" >&2; exit 1; }
[ -x "$STAGE_DIR/bin/wittymediaserver" ] || { echo "$STAGE_DIR has no server in bin." >&2; exit 1; }
case "$ARCH" in
  amd64|arm64) ;;
  *) echo "unknown arch $ARCH: there is a snap for amd64 and arm64" >&2; exit 2 ;;
esac
# A snap's version is at most 32 characters, of letters, digits and : . + ~ -
if [ ${#VERSION} -gt 32 ] || ! printf '%s' "$VERSION" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9:.+~-]*$'; then
  echo "$VERSION can't be a snap's version" >&2; exit 2
fi
case "$VERSION" in
  *-*) GRADE=devel ;;
  *) GRADE=stable ;;
esac
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
stage="$work/stage"
mkdir -p "$stage" "$work/snap/gui"
cp -R "$STAGE_DIR/bin" "$stage/"
[ -d "$STAGE_DIR/modules" ] && cp -R "$STAGE_DIR/modules" "$stage/"
cp -R "$STAGE_DIR/docs" "$stage/"
# Which kind of copy this is: one snapd looks after, and the server must not try to replace.
printf '{"product":"WittyMediaServer","kind":"snap"}\n' > "$stage/package-info.json"
cp "$STAGE_DIR/share/wittymediaserver.png" "$work/snap/gui/wittymediaserver.png"
sed -e "s|\${VERSION}|$VERSION|g" -e "s|\${GRADE}|$GRADE|g" packaging/linux/snap/snapcraft.yaml > "$work/snap/snapcraft.yaml"

target="$OUT/wittymediaserver-$VERSION-linux-$ARCH.snap"
rm -f "$target"
(cd "$work" && "$SNAPCRAFT" pack --build-for="$ARCH" --output "$target")
[ -s "$target" ] || { echo "snapcraft made no $target" >&2; exit 1; }
echo "   -> $target"
