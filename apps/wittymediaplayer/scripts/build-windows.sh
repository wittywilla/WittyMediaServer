#!/usr/bin/env bash
# Builds WittyMediaPlayer for Windows amd64 into a portable folder and a zip.
#
#   bash apps/wittymediaplayer/scripts/setup-toolchain.sh    (once)
#   bash apps/wittymediaplayer/scripts/build-windows.sh
#
# The result lands in apps/wittymediaplayer/.build/ , which Git ignores:
#   WittyMediaPlayer-<version>-windows-amd64/       run wittymediaplayer.exe from here
#   WittyMediaPlayer-<version>-windows-amd64.zip    the same folder, to share
#   source/                                         FFmpeg's source, to go beside the zip
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
T="${WMP_TOOLCHAIN:-$HOME/.wittymediaplayer-toolchain}"
QT_VERSION=6.10.3
FFMPEG_VERSION=8.1
QT="$T/qt/$QT_VERSION/mingw_64"
FF="$T/ffmpeg-min"
OUT="$HERE/.build"
VERSION="$(sed -n 's/^project(WittyMediaPlayer VERSION \([0-9.]*\).*/\1/p' "$HERE/CMakeLists.txt")"
PKG="$OUT/WittyMediaPlayer-$VERSION-windows-amd64"

[ -x "$QT/bin/qsb.exe" ] && [ -f "$FF/bin/avcodec-62.dll" ] || { echo "Run scripts/setup-toolchain.sh first." >&2; exit 1; }
export PATH="$T/qt/Tools/mingw1310_64/bin:$T/qt/Tools/Ninja:$T/qt/Tools/CMake_64/bin:$QT/bin:$FF/bin:$PATH"

# The helper: no cgo, no console window, and no record of where it was built.
mkdir -p "$OUT/cmake"
(cd "$HERE/engine" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H=windowsgui -X main.Version=$VERSION" -o "$OUT/cmake/wittymediaplayer-engine.exe" .)

# The window.
cmake -S "$HERE" -B "$OUT/cmake" -G Ninja -DCMAKE_BUILD_TYPE=Release -DCMAKE_PREFIX_PATH="$(cygpath -m "$QT")" -DFFMPEG_DIR="$(cygpath -m "$FF")" \
  -DCMAKE_EXE_LINKER_FLAGS="-s" >/dev/null
cmake --build "$OUT/cmake"

# The package: nothing in it but what's needed to run and what its licences ask for.
rm -rf "$PKG"
mkdir -p "$PKG/licenses"
cp "$OUT/cmake/wittymediaplayer.exe" "$OUT/cmake/wittymediaplayer-engine.exe" "$PKG/"
windeployqt6 --release --no-translations --no-system-d3d-compiler --no-opengl-sw --no-system-dxc-compiler --no-compiler-runtime \
  --skip-plugin-types generic,networkinformation,tls,qmltooling,iconengines --dir "$PKG" "$PKG/wittymediaplayer.exe" >"$OUT/windeployqt.log"
# The compiler's runtime libraries.
for dll in libgcc_s_seh-1.dll libstdc++-6.dll libwinpthread-1.dll; do cp "$T/qt/Tools/mingw1310_64/bin/$dll" "$PKG/"; done
# Qt's own FFmpeg media backend isn't used, and it's a general build with network code:
# this player decodes through the libraries built for it.
rm -rf "$PKG/multimedia" "$PKG/tls" "$PKG/networkinformation" "$PKG/generic"
rm -f "$PKG"/av*.dll "$PKG"/sw*.dll
for lib in avcodec-62 avformat-62 avfilter-11 avutil-60 swresample-6 swscale-9; do cp "$FF/bin/$lib.dll" "$PKG/"; done
rm -f "$PKG"/Qt6Svg.dll "$PKG"/dxcompiler.dll "$PKG"/dxil.dll
rm -f "$PKG"/imageformats/q{gif,icns,ico,pdf,svg,tga,tiff,wbmp,webp}*.dll

cp "$HERE/README.md" "$HERE/LICENSE" "$HERE/NOTICE" "$PKG/"
cp "$HERE/licenses/"* "$PKG/licenses/"

# LGPL: FFmpeg's source travels beside the binaries.
mkdir -p "$OUT/source"
cp "$T/downloads/ffmpeg-$FFMPEG_VERSION.tar.xz" "$OUT/source/"
sed -n '/DEC="/,/--extra-ldflags/p' "$HERE/scripts/setup-toolchain.sh" > "$OUT/source/how-ffmpeg-was-configured.txt"

bash "$HERE/scripts/check-package.sh" "$PKG"

rm -f "$PKG.zip"
(cd "$OUT" && powershell.exe -NoProfile -Command "Compress-Archive -Path '$(basename "$PKG")' -DestinationPath '$(basename "$PKG").zip'")
du -sh "$PKG" "$PKG.zip"
echo "Built $PKG"
