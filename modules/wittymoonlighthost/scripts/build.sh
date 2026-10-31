#!/bin/sh
# Builds this program, from fetching its sources to one file in dist/:
#
#   sh scripts/build.sh [windows-x64 | linux-x64 | linux-arm64 | macos-x64 | macos-arm64]
#
# With no target it builds for this computer. On Linux, windows-x64 and linux-arm64 cross-compile.
set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) native=linux-x64 ;;
  Linux-aarch64) native=linux-arm64 ;;
  Darwin-x86_64) native=macos-x64 ;;
  Darwin-arm64) native=macos-arm64 ;;
  *) native= ;;
esac
target="${1:-$native}"
[ -n "$target" ] || { echo "Name a target: this computer isn't one this script knows." >&2; exit 1; }

ext=
set --
case "$target" in
  macos-x64) set -- -DCMAKE_OSX_ARCHITECTURES=x86_64 -DCMAKE_OSX_DEPLOYMENT_TARGET=11.0 ;;
  macos-arm64) set -- -DCMAKE_OSX_ARCHITECTURES=arm64 -DCMAKE_OSX_DEPLOYMENT_TARGET=11.0 ;;
  windows-x64)
    ext=.exe
    set -- "-DCMAKE_TOOLCHAIN_FILE=$ROOT/cmake/windows-x64.cmake"
    ;;
  "$native")
    # On Alpine the C library is musl, which can be built in: the file then runs on any Linux.
    if [ -f /etc/alpine-release ]; then set -- -DWITTY_STATIC=ON; fi
    ;;
  linux-arm64) set -- "-DCMAKE_TOOLCHAIN_FILE=$ROOT/cmake/linux-arm64.cmake" ;;
  *)
    echo "usage: sh scripts/build.sh [windows-x64 | linux-x64 | linux-arm64 | macos-x64 | macos-arm64]" >&2
    exit 1
    ;;
esac

sh "$ROOT/scripts/fetch-core.sh"
cmake -S "$ROOT" -B "$ROOT/build/$target" -DCMAKE_BUILD_TYPE=Release "$@"
cmake --build "$ROOT/build/$target" --parallel
mkdir -p "$ROOT/dist"
cp "$ROOT/build/$target/wittymoonlight-host$ext" "$ROOT/dist/wittymoonlight-host-$target$ext"
case "$target" in macos-*) strip "$ROOT/dist/wittymoonlight-host-$target$ext" ;; esac
echo "$ROOT/dist/wittymoonlight-host-$target$ext"
