#!/usr/bin/env bash
# Checks a built package against the rules it must keep. Stops the build if any fail.
#
#   bash scripts/check-package.sh <package folder>
set -euo pipefail
PKG="$1"
fail=0
say() { echo "check-package: $*" >&2; fail=1; }

# Nothing may carry or load a disc-decryption library or a key database.
for pattern in libdvdcss dvdcss libaacs libbdplus libmmbd KEYDB keydb.cfg libbluray libdvdread libdvdnav; do
  if grep -lai -- "$pattern" "$PKG"/*.dll "$PKG"/*.exe >/dev/null 2>&1; then
    for f in $(grep -lai -- "$pattern" "$PKG"/*.dll "$PKG"/*.exe); do say "$pattern is mentioned in $(basename "$f")"; done
  fi
done
for f in "$PKG"/*.dll; do
  case "$(basename "$f" | tr A-Z a-z)" in
    *dvdcss*|*aacs*|*bdplus*|*bluray*|*dvdread*|*dvdnav*) say "$(basename "$f") must not ship" ;;
  esac
done

# Nothing may reach the network.
# Qt's sound library won't load without Qt6Network.dll, so it ships, but the player must
# not link to it itself, and Qt's network plugins stay out.
if strings_of() { grep -qa -- "$1" "$2"; }; strings_of "Qt6Network.dll" "$PKG/wittymediaplayer.exe"; then say "the player must not link to Qt6Network"; fi
for d in tls networkinformation; do [ ! -e "$PKG/$d" ] || say "Qt's $d plugins must not ship"; done
for f in "$PKG"/av*.dll; do
  if grep -qa -- "http_protocol\|tcp_protocol\|tls_protocol" "$f"; then say "$(basename "$f") has network code"; fi
done
if grep -qa -- "--enable-network" "$PKG"/avformat-*.dll; then say "FFmpeg was built with its network code"; fi

# FFmpeg must be the LGPL build.
if grep -qa -- "--enable-gpl\|--enable-nonfree" "$PKG"/avcodec-*.dll; then say "FFmpeg must not be built with GPL or non-free parts"; fi

# The licences have to go with it.
for f in LICENSE NOTICE README.md licenses/THIRD-PARTY.txt licenses/LGPL-3.0.txt licenses/GPL-3.0.txt; do
  [ -s "$PKG/$f" ] || say "$f is missing"
done

[ "$fail" = 0 ] && echo "check-package: all good"
exit "$fail"
