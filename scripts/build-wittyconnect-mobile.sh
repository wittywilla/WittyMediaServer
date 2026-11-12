#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Builds the peer-to-peer connector (pkg/wittyconnect/mobile) as a library for the phone and
# TV apps, with gomobile: an .aar for Android, an .xcframework for Apple (iPhone, iPad and
# Apple TV, each with its simulator). docs/P2P.md says how the apps use it.
#
#   scripts/build-wittyconnect-mobile.sh android   # needs the Android SDK and NDK
#   scripts/build-wittyconnect-mobile.sh apple     # needs Xcode; on a Mac only
#   scripts/build-wittyconnect-mobile.sh all
#
# Output goes to dist/wittyconnect/ (or the folder in OUT), with THIRD-PARTY-NOTICES.txt:
# every library inside it and each license's text, which has to ship with the app.
#
# The tools are pinned and kept to themselves. Nothing is installed into ~/go/bin: each
# gomobile goes into .tools/ in this checkout (or the folder in TOOLS), under its version.
#   Android: golang.org/x/mobile, the Go project's own (BSD-3-Clause).
#   Apple:   github.com/sagernet/gomobile (BSD-3-Clause, a fork of the above), because the Go
#            project's gomobile has no Apple TV target and this one does (-target tvos,
#            tvossimulator). It is the one sing-box ships its Apple TV app with.
#
# gomobile needs its bind package among a module's requirements. So that the server's own
# go.mod doesn't carry something only this build uses, the build happens in a scratch module
# that points at this checkout.
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
# The oldest systems the apps support (WittyMediaClient's android/app/build.gradle.kts and apple/project.yml).
android_api="${ANDROID_API:-23}"
ios_version="${IOS_VERSION:-16.0}"
tvos_version="${TVOS_VERSION:-17.0}"
# Which Apple systems get a slice. Add macos or maccatalyst here if something needs them.
apple_targets="${APPLE_TARGETS:-ios,iossimulator,tvos,tvossimulator}"

# The pinned tools. To move to a newer one, change the version here and build again.
xmobile_module=golang.org/x/mobile
xmobile_version=v0.0.0-20261011034528-197770444867
sagernet_module=github.com/sagernet/gomobile
sagernet_version=v0.1.13 # commit 9f03b8f25789099c5c8abef4a02085da783ba923

tools="${TOOLS:-$repo/.tools}"
mkdir -p "$tools"
tools="$(cd "$tools" && pwd)"

# install_gomobile <module> <version> prints the folder that holds that gomobile and its gobind.
install_gomobile() {
  local bin
  bin="$tools/gomobile/$(basename "$(dirname "$1")")-$(basename "$1")@$2/bin"
  if [ ! -x "$bin/gomobile" ] || [ ! -x "$bin/gobind" ]; then
    echo "Installing $1 $2 into $bin" >&2
    (cd "$tools" && GOBIN="$bin" GOFLAGS=-mod=mod GOWORK=off go install "$1/cmd/gomobile@$2" "$1/cmd/gobind@$2") >&2
  fi
  echo "$bin"
}

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

# scratch_module <module> <version> makes, in a folder of its own, a module with the same
# versions of everything as the server is built with: its go.mod and go.sum, with the server
# itself pointed at this checkout and gomobile's bind package added. It prints the folder.
scratch_module() {
  local dir
  dir="$scratch/$(basename "$(dirname "$1")")"
  mkdir -p "$dir"
  cat > "$dir/go.mod" <<MOD
module wittyconnect-mobile-build

go $(cd "$repo" && go list -m -f '{{.GoVersion}}')

require github.com/wittywilla/wittymediaserver v0.0.0

replace github.com/wittywilla/wittymediaserver => $repo
MOD
  cat > "$dir/tools.go" <<TOOLS
//go:build tools

package tools

import (
	_ "github.com/wittywilla/wittymediaserver/pkg/wittyconnect/mobile"
	_ "$1/bind"
)
TOOLS
  cp "$repo/go.sum" "$dir/go.sum"
  (cd "$dir" && GOFLAGS=-mod=mod GOWORK=off go get "$1/bind@$2" >/dev/null && GOFLAGS=-mod=mod GOWORK=off go mod tidy >/dev/null) >&2
  # The part of gomobile that ends up inside the library: its license goes in the notices.
  local src
  src="$(cd "$dir" && GOFLAGS=-mod=mod GOWORK=off go list -m -f '{{.Dir}}' "$1")"
  {
    printf '\n\n================================================================================\n'
    printf '%s %s: gomobile, whose bind package joins Go to Kotlin and Swift\nBSD-3-Clause\n' "$1" "$2"
    printf '================================================================================\n\n'
    cat "$src/LICENSE"
  } >> "$scratch/gomobile-notices.txt"
  echo "$dir"
}

export GOWORK=off GOFLAGS=-mod=mod
flags=(-trimpath -ldflags "-s -w")

if [ "$what" = android ] || [ "$what" = all ]; then
  : "${ANDROID_HOME:?set ANDROID_HOME to the Android SDK}"
  if [ -z "${ANDROID_NDK_HOME:-}" ]; then
    ANDROID_NDK_HOME="$(ls -d "$ANDROID_HOME"/ndk/* 2>/dev/null | sort -V | tail -1 || true)"
    export ANDROID_NDK_HOME
  fi
  [ -d "${ANDROID_NDK_HOME:-/nonexistent}" ] || { echo "No Android NDK found. Install one (sdkmanager 'ndk;27.2.12479018') or set ANDROID_NDK_HOME." >&2; exit 1; }
  bin="$(install_gomobile "$xmobile_module" "$xmobile_version")"
  dir="$(scratch_module "$xmobile_module" "$xmobile_version")"
  echo "Building the Android library (API $android_api and newer)"
  # libp2p lists the network interfaces on Android 11 and later through github.com/wlynxg/anet, which reaches into
  # the standard library's net package by go:linkname. Go 1.23 and later refuse that unless the linker is told not
  # to check, as anet's own README says. Only the Android library needs it. Every processor the app ships an APK
  # for gets a slice, 32-bit x86 included.
  (cd "$dir" && PATH="$bin:$PATH" gomobile init && PATH="$bin:$PATH" gomobile bind -trimpath -ldflags "-s -w -checklinkname=0" \
    -target=android/arm64,android/arm,android/amd64,android/386 -androidapi "$android_api" \
    -javapkg com.wittywilla.wittyconnect -o "$out/wittyconnect.aar" "$pkg")
  ls -lh "$out/wittyconnect.aar"
fi

if [ "$what" = apple ] || [ "$what" = all ]; then
  [ "$(uname)" = Darwin ] || { echo "The Apple framework can only be built on a Mac." >&2; exit 1; }
  bin="$(install_gomobile "$sagernet_module" "$sagernet_version")"
  dir="$(scratch_module "$sagernet_module" "$sagernet_version")"
  echo "Building the Apple framework ($apple_targets; iOS $ios_version and tvOS $tvos_version and newer)"
  rm -rf "$out/Wittyconnect.xcframework"
  (cd "$dir" && PATH="$bin:$PATH" gomobile bind "${flags[@]}" -target="$apple_targets" \
    -iosversion "$ios_version" -tvosversion "$tvos_version" \
    -prefix Witty -o "$out/Wittyconnect.xcframework" "$pkg")
  # Every slice has to say which system it is for, or Xcode refuses to link it into that app.
  for slice in "$out/Wittyconnect.xcframework"/*/; do
    name="$(basename "$slice")"
    lib="$slice/Wittyconnect.framework/Wittyconnect"
    [ -f "$lib" ] || { echo "The $name slice has no library in it." >&2; exit 1; }
    want="${name%%-*}"
    for arch in $(lipo -archs "$lib"); do
      objs="$scratch/$name-$arch"
      mkdir -p "$objs"
      lipo -thin "$arch" "$lib" -output "$objs.a" 2>/dev/null || cp "$lib" "$objs.a"
      (cd "$objs" && ar x "$objs.a")
      got="$(for o in "$objs"/*.o; do vtool -show-build "$o" 2>/dev/null || true; done | awk '/platform/ {print tolower($2)}' | sort -u | tr '\n' ' ')"
      rm -rf "$objs" "$objs.a"
      case "$want:$got" in
        (ios:"ios "|ios:"iossimulator "|tvos:"tvos "|tvos:"tvossimulator "|macos:"macos "|ios:"maccatalyst ") ;;
        (*) echo "The $name slice ($arch) is marked for '$got', not for $want." >&2; exit 1 ;;
      esac
    done
    echo "  $name: $(lipo -archs "$lib"), $(du -sh "$slice" | cut -f1)"
  done
  du -sh "$out/Wittyconnect.xcframework"
fi

# What has to ship beside it.
(cd "$repo" && go run ./internal/licenses/gen -notices ./pkg/wittyconnect/mobile "$out/THIRD-PARTY-NOTICES.txt")
cat "$scratch/gomobile-notices.txt" >> "$out/THIRD-PARTY-NOTICES.txt"
echo "Done: $out"
