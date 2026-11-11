#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Builds the peer-to-peer connector (pkg/wittyconnect/mobile) as a library for the phone and
# TV apps, with gomobile: an .aar for Android, an .xcframework for Apple. docs/P2P.md says
# how the apps use it.
#
#   scripts/build-wittyconnect-mobile.sh android   # needs the Android SDK and NDK
#   scripts/build-wittyconnect-mobile.sh apple     # needs Xcode; on a Mac only
#   scripts/build-wittyconnect-mobile.sh all
#
# Output goes to dist/wittyconnect/ (or the folder in OUT), with THIRD-PARTY-NOTICES.txt:
# every library inside it and each license's text, which has to ship with the app.
#
# gomobile needs golang.org/x/mobile among a module's requirements. So that the server's
# own go.mod doesn't carry something only this build uses, the build happens in a scratch
# module that points at this checkout.
set -euo pipefail

what="${1:-}"
case "$what" in
  (android|apple|all) ;;
  (*) echo "usage: $0 android|apple|all" >&2; exit 2 ;;
esac

repo="$(cd "$(dirname "$0")/.." && pwd)"
out="${OUT:-$repo/dist/wittyconnect}"
mkdir -p "$out"
out="$(cd "$out" && pwd)"
pkg=github.com/wittywilla/wittymediaserver/pkg/wittyconnect/mobile
# The oldest systems the apps support.
android_api="${ANDROID_API:-24}"
ios_version="${IOS_VERSION:-15.0}"

gobin="$(go env GOPATH)/bin"
export PATH="$gobin:$PATH"
if ! command -v gomobile >/dev/null 2>&1; then
  echo "Installing gomobile into $gobin"
  go install golang.org/x/mobile/cmd/gomobile@latest
  go install golang.org/x/mobile/cmd/gobind@latest
fi

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
cd "$scratch"
# The same versions of everything as the server is built with: its go.mod and go.sum, with
# the server itself pointed at this checkout and x/mobile added.
cat > go.mod <<MOD
module wittyconnect-mobile-build

go $(cd "$repo" && go list -m -f '{{.GoVersion}}')

require github.com/wittywilla/wittymediaserver v0.0.0

replace github.com/wittywilla/wittymediaserver => $repo
MOD
cat > tools.go <<'TOOLS'
//go:build tools

package tools

import (
	_ "github.com/wittywilla/wittymediaserver/pkg/wittyconnect/mobile"
	_ "golang.org/x/mobile/bind"
)
TOOLS
cp "$repo/go.sum" go.sum
GOFLAGS=-mod=mod GOWORK=off go get golang.org/x/mobile/bind@latest >/dev/null
GOFLAGS=-mod=mod GOWORK=off go mod tidy >/dev/null
export GOWORK=off GOFLAGS=-mod=mod
gomobile init

flags=(-trimpath -ldflags "-s -w")

if [ "$what" = android ] || [ "$what" = all ]; then
  : "${ANDROID_HOME:?set ANDROID_HOME to the Android SDK}"
  if [ -z "${ANDROID_NDK_HOME:-}" ]; then
    ANDROID_NDK_HOME="$(ls -d "$ANDROID_HOME"/ndk/* 2>/dev/null | sort -V | tail -1 || true)"
    export ANDROID_NDK_HOME
  fi
  [ -d "${ANDROID_NDK_HOME:-/nonexistent}" ] || { echo "No Android NDK found. Install one (sdkmanager 'ndk;27.2.12479018') or set ANDROID_NDK_HOME." >&2; exit 1; }
  echo "Building the Android library (API $android_api and newer)"
  gomobile bind "${flags[@]}" -target=android/arm64,android/arm,android/amd64 -androidapi "$android_api" \
    -javapkg com.wittywilla.wittyconnect -o "$out/wittyconnect.aar" "$pkg"
  ls -lh "$out/wittyconnect.aar"
fi

if [ "$what" = apple ] || [ "$what" = all ]; then
  [ "$(uname)" = Darwin ] || { echo "The Apple framework can only be built on a Mac." >&2; exit 1; }
  echo "Building the Apple framework (iOS $ios_version and newer, the simulator, macOS)"
  rm -rf "$out/Wittyconnect.xcframework"
  gomobile bind "${flags[@]}" -target=ios,iossimulator,macos -iosversion "$ios_version" \
    -prefix Witty -o "$out/Wittyconnect.xcframework" "$pkg"
  du -sh "$out/Wittyconnect.xcframework"
fi

# What has to ship beside it.
(cd "$repo" && go run ./internal/licenses/gen -notices ./pkg/wittyconnect/mobile "$out/THIRD-PARTY-NOTICES.txt")
echo "Done: $out"
