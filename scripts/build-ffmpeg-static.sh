#!/bin/sh
# Builds a fully static FFmpeg + FFprobe (GPL: x264, mp3, opus) from source.
# Used for FreeBSD, where no prebuilt static binaries exist, and usable on
# Linux/macOS too. Output: $OUT/ffmpeg, $OUT/ffprobe
#
#   FreeBSD prerequisites:  pkg install -y bash gmake nasm pkgconf curl git
#   Debian/Ubuntu:          apt install -y build-essential nasm pkg-config curl xz-utils git
#
# Build output is kept quiet; on failure the last lines of the failing step's
# log are printed.
#   usage: scripts/build-ffmpeg-static.sh [out-dir]
set -eu
OUT="${1:-$(pwd)/.tools/ffmpeg-static}"
WORK="${WORK:-$(pwd)/.tools/ffmpeg-src}"
JOBS="${JOBS:-$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)}"
FFMPEG_VER="${FFMPEG_VER:-7.1.1}"
LAME_VER=3.100
OPUS_VER=1.5.2
MAKE=make; command -v gmake >/dev/null 2>&1 && MAKE=gmake
# FreeBSD (and macOS) ship clang as "cc" and no gcc, but x264 and FFmpeg
# default to "gcc", so name the compiler explicitly everywhere.
CC="${CC:-cc}"; export CC
# Errors are also printed as GitHub Actions annotations ("::error"), which are
# readable without signing in (the build runs inside a FreeBSD VM over ssh,
# so the usual CI environment variables are not visible here).
note_error() {
  echo "$1" >&2
  printf '::error title=%s::%s\n' "$2" "$(printf '%s' "$1" | sed 's/%/%25/g' | awk 'BEGIN{ORS="%0A"} {print}')"
}
BASH_BIN="$(command -v bash || true)"
PKGCFG=pkg-config; command -v pkg-config >/dev/null 2>&1 || PKGCFG=pkgconf
for tool in "$MAKE" "$CC" curl git tar "$PKGCFG"; do
  command -v "$tool" >/dev/null 2>&1 || { note_error "missing tool: $tool" "ffmpeg build"; exit 1; }
done
[ -n "$BASH_BIN" ] || { note_error "missing tool: bash (needed by x264's configure)" "ffmpeg build"; exit 1; }
case "$(uname -m)" in amd64|x86_64) command -v nasm >/dev/null 2>&1 || { note_error "missing tool: nasm" "ffmpeg build"; exit 1; } ;; esac
echo "==> $(uname -sr) $(uname -m), $CC: $("$CC" --version 2>&1 | head -1), $JOBS jobs"
mkdir -p "$OUT" "$WORK" "$WORK/prefix"
PREFIX="$WORK/prefix"
export PKG_CONFIG_PATH="$PREFIX/lib/pkgconfig"
export CFLAGS="-O2 -fPIC -I$PREFIX/include"
export LDFLAGS="-L$PREFIX/lib"
cd "$WORK"

dl() { [ -s "$2" ] || curl -fL --retry 3 -o "$2" "$1"; }
# step <name> <command...>: runs quietly, shows the log tail when it fails.
step() {
  name="$1"; shift
  if ! "$@" >"$WORK/$name.log" 2>&1; then
    log="$WORK/$name.log"
    # configure scripts keep the real reason in config.log
    [ -f config.log ] && case "$name" in *configure) log="config.log" ;; esac
    [ -f ffbuild/config.log ] && case "$name" in *configure) log="ffbuild/config.log" ;; esac
    note_error "$name failed: $(tail -n 25 "$WORK/$name.log"; [ "$log" != "$WORK/$name.log" ] && { echo "--- $log:"; tail -n 25 "$log"; })" "$name failed"
    exit 1
  fi
}

echo "==> x264"
if [ ! -d x264 ]; then git clone --depth 1 https://code.videolan.org/videolan/x264.git x264; fi
cd x264
step x264-configure "$BASH_BIN" ./configure --prefix="$PREFIX" --enable-static --disable-shared --disable-cli --enable-pic
step x264-make $MAKE -j"$JOBS"
step x264-install $MAKE install
cd "$WORK"

echo "==> lame"
dl "https://downloads.sourceforge.net/project/lame/lame/$LAME_VER/lame-$LAME_VER.tar.gz" lame.tar.gz
[ -d "lame-$LAME_VER" ] || tar -xzf lame.tar.gz
cd "lame-$LAME_VER"
step lame-configure sh ./configure --prefix="$PREFIX" --enable-static --disable-shared --disable-frontend
step lame-make $MAKE -j"$JOBS"
step lame-install $MAKE install
cd "$WORK"

echo "==> opus"
dl "https://downloads.xiph.org/releases/opus/opus-$OPUS_VER.tar.gz" opus.tar.gz
[ -d "opus-$OPUS_VER" ] || tar -xzf opus.tar.gz
cd "opus-$OPUS_VER"
step opus-configure sh ./configure --prefix="$PREFIX" --enable-static --disable-shared --disable-doc --disable-extra-programs
step opus-make $MAKE -j"$JOBS"
step opus-install $MAKE install
cd "$WORK"

echo "==> ffmpeg $FFMPEG_VER"
dl "https://ffmpeg.org/releases/ffmpeg-$FFMPEG_VER.tar.xz" ffmpeg.tar.xz
[ -d "ffmpeg-$FFMPEG_VER" ] || tar -xJf ffmpeg.tar.xz
cd "ffmpeg-$FFMPEG_VER"
EXTRA_LD="-static"
[ "$(uname -s)" = Darwin ] && EXTRA_LD="" # macOS cannot link fully static; libs are still static
# Only the libraries built above plus libc/libm/libz are linked. Autodetected
# system libraries such as liblzma pull in further static dependencies
# (libmd on FreeBSD) and aren't needed, so they are disabled.
step ffmpeg-configure sh ./configure --prefix="$PREFIX" --cc="$CC" --pkg-config="$PKGCFG" --pkg-config-flags="--static" --extra-cflags="$CFLAGS" --extra-ldflags="$LDFLAGS $EXTRA_LD" \
  --enable-gpl --enable-version3 --enable-static --disable-shared --disable-debug --disable-doc --disable-ffplay \
  --enable-libx264 --enable-libmp3lame --enable-libopus --disable-vaapi --disable-vdpau --disable-xlib --disable-sdl2 \
  --disable-lzma --disable-bzlib --disable-iconv \
  --enable-pthreads
step ffmpeg-make $MAKE -j"$JOBS"
cp ffmpeg ffprobe "$OUT/"
strip "$OUT/ffmpeg" "$OUT/ffprobe" 2>/dev/null || true
echo "==> static ffmpeg in $OUT"
"$OUT/ffmpeg" -hide_banner -version | head -1
