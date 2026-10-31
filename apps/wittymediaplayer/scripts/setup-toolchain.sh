#!/usr/bin/env bash
# Gets everything WittyMediaPlayer's Windows build needs, for this user only: no
# administrator rights, no accounts, nothing installed system-wide.
#
#   bash apps/wittymediaplayer/scripts/setup-toolchain.sh
#
# Needs Git for Windows (for this shell) and Python 3. It takes about ten minutes
# and 4 GB. Everything lands in WMP_TOOLCHAIN (default ~/.wittymediaplayer-toolchain);
# delete that folder to remove it all.
set -euo pipefail

QT_VERSION=6.10.3
FFMPEG_VERSION=8.1
FFMPEG_SHA256=b072aed6871998cce9b36e7774033105ca29e33632be5b6347f3206898e0756a
ZLIB_VERSION=1.3.1
ZLIB_SHA256=9a93b2b7dfdac77ceba5a558a580e74667dd6fede4585b91eefb60f03b72df23
NASM_VERSION=2.16.03
NASM_SHA256=3ee4782247bcb874378d02f7eab4e294a84d3d15f3f6ee2de2f47a46aa7226e6

T="${WMP_TOOLCHAIN:-$HOME/.wittymediaplayer-toolchain}"
mkdir -p "$T/downloads" "$T/src" "$T/tmp"
# FFmpeg's configure can't use a temporary folder written with backslashes.
export TMPDIR="$T/tmp"

fetch() { # <url> <file> <sha256>
  local out="$T/downloads/$2"
  [ -s "$out" ] || curl -fsSL --retry 3 -o "$out" "$1"
  if [ "$3" != "skip" ]; then
    echo "$3  $out" | sha256sum -c --quiet - || { echo "$2 is not the file this script expects; stopping." >&2; exit 1; }
  fi
}

# Qt, its MinGW compiler, CMake and Ninja, from Qt's own servers.
if [ ! -x "$T/qt/$QT_VERSION/mingw_64/bin/qsb.exe" ]; then
  [ -x "$T/aqt-venv/Scripts/aqt" ] || { python -m venv "$T/aqt-venv"; "$T/aqt-venv/Scripts/python" -m pip install -q --disable-pip-version-check aqtinstall; }
  A="$T/aqt-venv/Scripts/aqt"
  "$A" install-qt windows desktop "$QT_VERSION" win64_mingw -O "$T/qt" -m qtmultimedia qtshadertools
  "$A" install-tool windows desktop tools_mingw1310 -O "$T/qt"
  "$A" install-tool windows desktop tools_cmake -O "$T/qt"
  "$A" install-tool windows desktop tools_ninja -O "$T/qt"
fi
export PATH="$T/qt/Tools/mingw1310_64/bin:$PATH"

# NASM assembles FFmpeg's fast paths.
if [ ! -x "$T/nasm/nasm.exe" ]; then
  fetch "https://www.nasm.us/pub/nasm/releasebuilds/$NASM_VERSION/win64/nasm-$NASM_VERSION-win64.zip" "nasm-$NASM_VERSION.zip" "$NASM_SHA256"
  mkdir -p "$T/nasm"
  unzip -qoj "$T/downloads/nasm-$NASM_VERSION.zip" "*/nasm.exe" -d "$T/nasm"
fi
export PATH="$T/nasm:$PATH"

# zlib: Matroska files often compress their subtitle tracks with it.
P="$T/ffmpeg-min"
if [ ! -f "$P/lib/libz.a" ]; then
  fetch "https://github.com/madler/zlib/releases/download/v$ZLIB_VERSION/zlib-$ZLIB_VERSION.tar.gz" "zlib-$ZLIB_VERSION.tar.gz" "$ZLIB_SHA256"
  rm -rf "$T/src/zlib-$ZLIB_VERSION"
  tar -xzf "$T/downloads/zlib-$ZLIB_VERSION.tar.gz" -C "$T/src"
  mkdir -p "$P/include" "$P/lib"
  (cd "$T/src/zlib-$ZLIB_VERSION" && mingw32-make -s -f win32/Makefile.gcc libz.a CFLAGS="-O2 -w" && cp libz.a "$P/lib/" && cp zlib.h zconf.h "$P/include/")
fi

# FFmpeg, built with only what a player needs. It is much smaller than a general
# build, has no network code at all, and leaves out every library that could load
# a disc-decryption library: there is no libbluray and no libdvdread here, and none
# may be added.
#
# LGPL: no --enable-gpl and no --enable-nonfree. Shared libraries, so anyone can
# replace them.
if [ ! -f "$P/bin/avcodec-62.dll" ]; then
  fetch "https://ffmpeg.org/releases/ffmpeg-$FFMPEG_VERSION.tar.xz" "ffmpeg-$FFMPEG_VERSION.tar.xz" "$FFMPEG_SHA256"
  rm -rf "$T/src/ffmpeg-$FFMPEG_VERSION"
  tar -xJf "$T/downloads/ffmpeg-$FFMPEG_VERSION.tar.xz" -C "$T/src"
  DEC="mpeg1video mpeg2video mpeg4 h263 h264 hevc vc1 wmv1 wmv2 wmv3 vp8 vp9 av1 theora msmpeg4v1 msmpeg4v2 msmpeg4v3 mjpeg
       aac aac_latm ac3 eac3 dca truehd mlp mp1 mp2 mp3 mp3float flac alac opus vorbis wmav1 wmav2 wmapro wmalossless
       pcm_s8 pcm_u8 pcm_s16le pcm_s16be pcm_s24le pcm_s24be pcm_s32le pcm_s32be pcm_f32le pcm_f64le pcm_alaw pcm_mulaw pcm_dvd pcm_bluray adpcm_ms ape tta wavpack
       dvdsub pgssub dvbsub xsub subrip srt ass ssa webvtt text mov_text"
  DMX="matroska mov avi mpegts mpegps mpegvideo mpegtsraw asf flv ogg wav flac mp3 aac ac3 eac3 dts dtshd truehd mlp ape wv aiff tta caf nut rm vc1 h264 hevc m4v
       srt ass webvtt"
  PAR="h264 hevc mpegvideo mpeg4video vc1 vp8 vp9 av1 aac aac_latm ac3 dca mlp mpegaudio flac opus vorbis dvdsub dvd_nav dvbsub pgs"
  BSF="aac_adtstoasc h264_mp4toannexb hevc_mp4toannexb extract_extradata vp9_superframe_split vp9_superframe av1_frame_merge av1_frame_split dts2pts truehd_core"
  list() { echo $1 | tr ' ' ','; }
  (
    cd "$T/src/ffmpeg-$FFMPEG_VERSION"
    ./configure --prefix="$P" --arch=x86_64 --target-os=mingw32 --cc=gcc --cxx=g++ --x86asmexe=nasm \
      --enable-shared --disable-static --enable-version3 --enable-w32threads --disable-pthreads \
      --disable-autodetect --disable-network --disable-programs --disable-doc --disable-debug --disable-avdevice \
      --disable-everything --enable-zlib --enable-d3d11va --enable-dxva2 \
      --enable-decoder="$(list "$DEC")" --enable-demuxer="$(list "$DMX")" --enable-parser="$(list "$PAR")" \
      --enable-bsf="$(list "$BSF")" --enable-protocol=file \
      --enable-hwaccel=h264_d3d11va,h264_d3d11va2,hevc_d3d11va,hevc_d3d11va2,vc1_d3d11va,vc1_d3d11va2,wmv3_d3d11va,wmv3_d3d11va2,mpeg2_d3d11va,mpeg2_d3d11va2,vp9_d3d11va,vp9_d3d11va2,av1_d3d11va,av1_d3d11va2 \
      --enable-filter=yadif,bwdif,buffer,buffersink,format,scale \
      --extra-cflags="-I$(cygpath -m "$P/include")" --extra-ldflags="-L$(cygpath -m "$P/lib") -static-libgcc" \
      >"$T/ffmpeg-configure.log" 2>&1 || { tail -20 "$T/ffmpeg-configure.log"; tail -30 ffbuild/config.log; exit 1; }
    # Windows can't carry FFmpeg's long lists of object files on a command line, so
    # they travel through files. This changes how the libraries are linked, not what's
    # in them.
    sed -i -e 's|\$(Q)echo \$\$(filter %.o,\$\$^) > \$\$@.objs|$$(file >$$@.objs,$$(filter %.o,$$^))|' ffbuild/library.mak
    sed -i -e 's|^SLIB_CREATE_DEF_CMD=\(.*\)\$(OBJS) >|SLIB_CREATE_DEF_CMD=$$(file >$$@.defobjs,$(OBJS)) \1 @$$@.defobjs >|' ffbuild/config.mak
    sed -i -e 's|^shift$|shift\ncase "$1" in @*) list=${1#@}; set -- $(cat "$list");; esac|' compat/windows/makedef
    mingw32-make SHELL=sh RESPONSE_FILES=yes -s -j"$(nproc)" >"$T/ffmpeg-build.log" 2>&1 || { tail -30 "$T/ffmpeg-build.log"; exit 1; }
    mingw32-make SHELL=sh RESPONSE_FILES=yes -s install >>"$T/ffmpeg-build.log" 2>&1
  )
  grep -i "^License" "$T/ffmpeg-configure.log" || true
fi

# A general FFmpeg for making test clips only. It is never packaged.
if [ ! -x "$T/test-ffmpeg/ffmpeg.exe" ]; then
  fetch "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-n8.1-latest-win64-lgpl-8.1.zip" "test-ffmpeg.zip" skip
  mkdir -p "$T/test-ffmpeg"
  unzip -qoj "$T/downloads/test-ffmpeg.zip" "*/bin/ffmpeg.exe" "*/bin/ffprobe.exe" -d "$T/test-ffmpeg"
fi

echo "Toolchain ready in $T"
