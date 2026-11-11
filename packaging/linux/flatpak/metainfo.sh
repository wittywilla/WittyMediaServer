#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Prints the AppStream description with a version and today's date in it, for the Flatpak and
# the AppImage.
#
#   packaging/linux/flatpak/metainfo.sh <version>
set -eu
VERSION="${1:?version}"
# AppStream reads 0.8.0~alpha as coming before 0.8.0, and anything with a suffix is a
# development release.
case "$VERSION" in
  *-*) TYPE=development ;;
  *) TYPE=stable ;;
esac
STREAM_VERSION="$(printf '%s' "$VERSION" | sed 's/-/~/')"
sed -e "s|@VERSION@|$STREAM_VERSION|g" -e "s|@RELEASE_TYPE@|$TYPE|g" -e "s|@DATE@|$(date -u +%Y-%m-%d)|g" \
  "$(dirname "$0")/com.wittywilla.WittyMediaServer.metainfo.xml"
