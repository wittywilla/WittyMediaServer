#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Signs WittyMediaServer.app and Uninstall WittyMediaServer.app, from the inside out. Runs on a
# Mac only: codesign is Apple's.
#
#   packaging/macos/sign-app.sh <folder holding the apps> [identity]
#
# identity is a "Developer ID Application: ..." name, or - for an ad hoc signature (the
# default). Ad hoc is still worth it: Local Network privacy knows a program by the name in its
# signature, and the one the linker gives every Go program is "a.out". Signing the app also
# seals its Info.plist, so macOS can read why it asks for the local network.
set -euo pipefail

dir="${1:?folder holding the apps}"; id="${2:--}"
app="$dir/WittyMediaServer.app"; un="$dir/Uninstall WittyMediaServer.app"
[ -d "$app/Contents/Helpers" ] || { echo "$dir has no WittyMediaServer.app with its helpers." >&2; exit 1; }
opts=(--force --sign "$id")
# What notarising asks for. An ad hoc signature can't have a timestamp.
[ "$id" != - ] && opts+=(--options runtime --timestamp)

for f in "$app/Contents/Helpers/"*; do
  name="$(basename "$f")"
  case "$name" in
    wittymediaserver) ident=com.wittywilla.wittymediaserver.server ;;
    *)
      # FFmpeg comes signed by whoever built it. Without an identity of our own, that's better
      # than ad hoc.
      if [ "$id" = - ] && codesign --verify "$f" 2>/dev/null && ! codesign -dv "$f" 2>&1 | grep -q linker-signed; then
        continue
      fi
      ident="com.wittywilla.wittymediaserver.$name" ;;
  esac
  codesign "${opts[@]}" --identifier "$ident" "$f"
done
# The apps take their names from CFBundleIdentifier.
codesign "${opts[@]}" "$app"
[ -d "$un" ] && codesign "${opts[@]}" "$un"
codesign --verify --strict "$app"
