#!/bin/sh
# Builds a fully static FFmpeg + FFprobe (GPL: x264, mp3, opus) from source.
# Used for FreeBSD, where no prebuilt static binaries exist, and usable on
# Linux/macOS too. Output: $OUT/ffmpeg, $OUT/ffprobe
#
#   FreeBSD prerequisites:  pkg install -y gmake nasm pkgconf curl
#   Debian/Ubuntu:          apt install -y build-essential nasm pkg-config curl xz-utils
#   usage: scripts/build-ffmpeg-static.sh [out-dir]
set -eu
OUT="${1:-$(pwd)/.tools/ffmpeg-static}"
WORK="${WORK:-$(pwd)/.tools/ffmpeg-src}"
JOBS="${JOBS:-$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)}"
FFMPEG_VER="${FFMPEG_VER:-7.1.1}"
LAME_VER=3.100
OPUS_VER=1.5.2
MAKE=make; command -v gmake >/dev/null 2>&1 && MAKE=gmake
mkdir -p "$OUT" "$WORK" "$WORK/prefix"
PREFIX="$WORK/prefix"
export PKG_CONFIG_PATH="$PREFIX/lib/pkgconfig"
export CFLAGS="-O2 -fPIC -I$PREFIX/include"
export LDFLAGS="-L$PREFIX/lib"
cd "$WORK"

dl() { [ -s "$2" ] || curl -fL --retry 3 -o "$2" "$1"; }

echo "==> x264"
if [ ! -d x264 ]; then git clone --depth 1 https://code.videolan.org/videolan/x264.git x264; fi
(cd x264 && ./configure --prefix="$PREFIX" --enable-static --disable-shared --disable-cli --enable-pic >/dev/null && $MAKE -j"$JOBS" >/dev/null && $MAKE install >/dev/null)

echo "==> lame"
dl "https://downloads.sourceforge.net/project/lame/lame/$LAME_VER/lame-$LAME_VER.tar.gz" lame.tar.gz
[ -d "lame-$LAME_VER" ] || tar -xzf lame.tar.gz
(cd "lame-$LAME_VER" && ./configure --prefix="$PREFIX" --enable-static --disable-shared --disable-frontend >/dev/null && $MAKE -j"$JOBS" >/dev/null && $MAKE install >/dev/null)

echo "==> opus"
dl "https://downloads.xiph.org/releases/opus/opus-$OPUS_VER.tar.gz" opus.tar.gz
[ -d "opus-$OPUS_VER" ] || tar -xzf opus.tar.gz
(cd "opus-$OPUS_VER" && ./configure --prefix="$PREFIX" --enable-static --disable-shared --disable-doc --disable-extra-programs >/dev/null && $MAKE -j"$JOBS" >/dev/null && $MAKE install >/dev/null)

echo "==> ffmpeg $FFMPEG_VER"
dl "https://ffmpeg.org/releases/ffmpeg-$FFMPEG_VER.tar.xz" ffmpeg.tar.xz
[ -d "ffmpeg-$FFMPEG_VER" ] || tar -xJf ffmpeg.tar.xz
cd "ffmpeg-$FFMPEG_VER"
EXTRA_LD="-static"
[ "$(uname -s)" = Darwin ] && EXTRA_LD="" # macOS cannot link fully static; libs are still static
./configure --prefix="$PREFIX" --pkg-config-flags="--static" --extra-cflags="$CFLAGS" --extra-ldflags="$LDFLAGS $EXTRA_LD" \
  --enable-gpl --enable-version3 --enable-static --disable-shared --disable-debug --disable-doc --disable-ffplay \
  --enable-libx264 --enable-libmp3lame --enable-libopus --disable-vaapi --disable-vdpau --disable-xlib --disable-sdl2 \
  --enable-pthreads >/dev/null
$MAKE -j"$JOBS" >/dev/null
cp ffmpeg ffprobe "$OUT/"
strip "$OUT/ffmpeg" "$OUT/ffprobe" 2>/dev/null || true
echo "==> static ffmpeg in $OUT"
"$OUT/ffmpeg" -hide_banner -version | head -1
