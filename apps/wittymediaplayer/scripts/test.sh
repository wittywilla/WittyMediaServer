#!/usr/bin/env bash
# Runs WittyMediaPlayer's tests with no one at the screen. It opens no windows and
# plays no sound: Qt's offscreen platform, and sound on a timer instead of a device.
#
#   WITTY_TEST_DVD=<image> WITTY_TEST_BD=<image> bash scripts/test.sh
#
# Discs named this way are only ever read.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
T="${WMP_TOOLCHAIN:-$HOME/.wittymediaplayer-toolchain}"
QT="$T/qt/6.10.3/mingw_64"
FF="$T/ffmpeg-min"
OUT="$HERE/.build"
export PATH="$T/qt/Tools/mingw1310_64/bin:$QT/bin:$FF/bin:$PATH"

echo "== Engine"
(cd "$HERE/engine" && go vet ./... && go test ./... -count=1)

echo "== Player"
[ -x "$OUT/cmake/wmptests.exe" ] || { echo "Build first: scripts/build-windows.sh" >&2; exit 1; }
QT_QPA_PLATFORM=offscreen WMP_TEST_FFMPEG="$T/test-ffmpeg/ffmpeg.exe" WMP_TEST_REPO="${WMP_TEST_REPO:-$HERE/../..}" \
  QT_PLUGIN_PATH="$(cygpath -m "$QT/plugins")" "$OUT/cmake/wmptests.exe" "$@"
