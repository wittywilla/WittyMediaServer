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
OGG_SHA256=0eb4b4b9420a0f51db142ba3f9c64b333f826532dc0f48c6410ae51f4799b664
VORBIS_SHA256=0e982409a9c3fc82ee06e08205b1355e5c6aa4c36bca58146ef399621b0ce5ab
OPUS_SHA256=65c1d2f78b9f2fb20082c38cbe47c951ad5839345876e46941612ee87f9a7ce1
LAME_SHA256=ddfe36cab873794038ae2c1210557ad34857a4b6bdc515785d1da9e175b1da1e
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

# What ripping CDs encodes with, all under licences that fit the LGPL route: LAME for MP3
# (LGPL), Opus and Vorbis (BSD). Built as static libraries inside FFmpeg's own, which stay
# replaceable as a whole. FLAC, ALAC, AAC and WAV are FFmpeg's own.
OGG_VERSION=1.3.5
VORBIS_VERSION=1.3.7
OPUS_VERSION=1.5.2
LAME_VERSION=3.100
if [ ! -f "$P/lib/libmp3lame.a" ] || [ ! -f "$P/lib/libopus.a" ] || [ ! -f "$P/lib/libvorbisenc.a" ]; then
  export PATH="$T/qt/Tools/CMake_64/bin:$T/qt/Tools/Ninja:$PATH"
  fetch "https://downloads.xiph.org/releases/ogg/libogg-$OGG_VERSION.tar.gz" "libogg-$OGG_VERSION.tar.gz" "$OGG_SHA256"
  fetch "https://downloads.xiph.org/releases/vorbis/libvorbis-$VORBIS_VERSION.tar.gz" "libvorbis-$VORBIS_VERSION.tar.gz" "$VORBIS_SHA256"
  fetch "https://github.com/xiph/opus/releases/download/v$OPUS_VERSION/opus-$OPUS_VERSION.tar.gz" "opus-$OPUS_VERSION.tar.gz" "$OPUS_SHA256"
  fetch "https://downloads.sourceforge.net/project/lame/lame/$LAME_VERSION/lame-$LAME_VERSION.tar.gz" "lame-$LAME_VERSION.tar.gz" "$LAME_SHA256"
  PM="$(cygpath -m "$P")"
  build_cmake() { # <folder> <extra args>
    local src="$T/src/$1"; shift
    cmake -S "$(cygpath -m "$src")" -B "$(cygpath -m "$src/b")" -G Ninja -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF \
      -DCMAKE_INSTALL_PREFIX="$PM" -DCMAKE_PREFIX_PATH="$PM" -DCMAKE_POLICY_VERSION_MINIMUM=3.5 "$@" >"$T/deps-build.log" 2>&1 || { tail -20 "$T/deps-build.log"; exit 1; }
    cmake --build "$(cygpath -m "$src/b")" --target install >>"$T/deps-build.log" 2>&1 || { tail -20 "$T/deps-build.log"; exit 1; }
  }
  for a in "libogg-$OGG_VERSION" "libvorbis-$VORBIS_VERSION" "opus-$OPUS_VERSION" "lame-$LAME_VERSION"; do
    rm -rf "$T/src/$a"
    tar -xzf "$T/downloads/$a.tar.gz" -C "$T/src"
  done
  build_cmake "libogg-$OGG_VERSION" -DINSTALL_DOCS=OFF
  build_cmake "libvorbis-$VORBIS_VERSION" -DOGG_ROOT="$PM"
  build_cmake "opus-$OPUS_VERSION" -DOPUS_BUILD_PROGRAMS=OFF -DOPUS_BUILD_TESTING=OFF
  # LAME has no CMake build: only the library is needed, from its plain sources.
  (
    cd "$T/src/lame-$LAME_VERSION"
    # Its Windows config spells out the integer types, which clash with this compiler's own.
    sed -E '/^#define (__int8_t_defined|u?int(8|16|32|64)_t)/d' configMS.h > config.h
    mkdir -p o "$P/include/lame"
    for f in libmp3lame/*.c libmp3lame/vector/xmm_quantize_sub.c mpglib/*.c; do
      gcc -O2 -w -msse2 -DHAVE_CONFIG_H -DHAVE_XMMINTRIN_H -DHAVE_STDINT_H -DTAKEHIRO_IEEE754_HACK -I. -Iinclude -Ilibmp3lame -Impglib -c "$f" -o "o/$(echo "$f" | tr / _).o" || exit 1
    done
    ar rcs "$P/lib/libmp3lame.a" o/*.o
    cp include/lame.h "$P/include/lame/"
  ) >>"$T/deps-build.log" 2>&1 || { tail -20 "$T/deps-build.log"; exit 1; }
fi
PM="$(cygpath -m "$P")"
# FFmpeg asks pkg-config about Opus and Vorbis; this answers for what was just built, and
# there's no need to install the real one.
mkdir -p "$T/pkgconfig-shim"
cat >"$T/pkgconfig-shim/pkg-config" <<SHIM
#!/bin/sh
case "\$*" in
  *--exists*|*--atleast-version*|*--print-errors*) exit 0 ;;
  *--version*|*--modversion*) echo 9.9.9; exit 0 ;;
  *--cflags*opus*) echo "-I$PM/include/opus"; exit 0 ;;
  *--cflags*) echo "-I$PM/include"; exit 0 ;;
  *--libs*opus*) echo "-L$PM/lib -lopus"; exit 0 ;;
  *--libs*vorbisenc*) echo "-L$PM/lib -lvorbisenc -lvorbis -logg"; exit 0 ;;
  *--libs*vorbis*) echo "-L$PM/lib -lvorbis -logg"; exit 0 ;;
  *--libs*ogg*) echo "-L$PM/lib -logg"; exit 0 ;;
esac
exit 1
SHIM
chmod +x "$T/pkgconfig-shim/pkg-config"

# FFmpeg, built with only what a player needs. It is much smaller than a general
# build, has no network code at all, and leaves out every library that could load
# a disc-decryption library: there is no libbluray and no libdvdread here, and none
# may be added.
#
# LGPL: no --enable-gpl and no --enable-nonfree. Shared libraries, so anyone can
# replace them.
if [ ! -f "$P/bin/avcodec-62.dll" ] || [ ! -f "$P/.with-encoders" ]; then
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
      --pkg-config="$T/pkgconfig-shim/pkg-config" --enable-libmp3lame --enable-libopus --enable-libvorbis \
      --enable-encoder=flac,alac,aac,pcm_s16le,libmp3lame,libopus,libvorbis --enable-muxer=flac,wav,ipod,mp4,mp3,ogg,opus \
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
    touch "$P/.with-encoders"
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
