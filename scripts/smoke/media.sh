#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Makes the smoke tests' media, as small as it can be: a 6-second film and three 4-second tracks
# made by FFmpeg, and one homebrew Game Boy game (Adjustris by tbsp, CC0 1.0, from its own
# release, checked against its SHA-256).
#   scripts/smoke/media.sh <folder>
set -euo pipefail
dir=${1:?where to make it}
mkdir -p "$dir/Movies/Smoke Film (2026)" "$dir/Music/Smoke Artist/Smoke Album" "$dir/Games/Game Boy"
ff() { ffmpeg -nostdin -hide_banner -loglevel error -y "$@"; }

film="$dir/Movies/Smoke Film (2026)/Smoke Film (2026).mp4"
# A film library takes anything under 5 MiB for a sample and leaves it out, so the picture is padded
# to a constant 2.5 Mbit/s: 20 seconds (long enough that a player's clock is still moving when a
# test looks twice), about 6.5 MB.
if [ "$(stat -c %s "$film" 2>/dev/null || echo 0)" -lt 5500000 ] || [ "$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$film" | cut -d. -f1)" -lt 20 ]; then
  ff -f lavfi -i testsrc2=size=640x360:rate=24:duration=20 -f lavfi -i sine=frequency=440:duration=20 \
    -c:v libx264 -preset ultrafast -pix_fmt yuv420p -b:v 2500k -minrate 2500k -maxrate 2500k -bufsize 1M -x264-params nal-hrd=cbr:filler=1 \
    -c:a aac -b:a 64k -movflags +faststart "$film"
fi

for i in 1 2 3; do
  t="$dir/Music/Smoke Artist/Smoke Album/0$i - Tone $i.mp3"
  [ -s "$t" ] || ff -f lavfi -i "sine=frequency=$((300 + i * 110)):duration=4" -c:a libmp3lame -b:a 64k \
    -metadata title="Tone $i" -metadata artist="Smoke Artist" -metadata album_artist="Smoke Artist" \
    -metadata album="Smoke Album" -metadata track="$i/3" -metadata date=2026 "$t"
done

rom="$dir/Games/Game Boy/adjustris.gb"
sum=b6c8affe6d906419cfc99ff459718f33a1868af03254a2f65cea2a9430394712
if [ ! -s "$rom" ] || [ "$(sha256sum "$rom" | cut -d' ' -f1)" != $sum ]; then
  curl -fsSL --retry 4 -o "$rom.part" https://github.com/tbsp/Adjustris/releases/download/v1.1/adjustris.gb
  [ "$(sha256sum "$rom.part" | cut -d' ' -f1)" = $sum ] || { echo "adjustris.gb: not the expected file" >&2; exit 1; }
  mv "$rom.part" "$rom"
fi
find "$dir" -type f | sort
