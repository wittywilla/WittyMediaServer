#!/bin/bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
#
# Makes a test library for scripts/perf out of FFmpeg's own test pictures and tones, so that
# runs compare from machine to machine (#643). Films are hard links to three 20-second clips,
# each in a folder of its own with a year; songs are short MP3s, each with its own tags.
#
#   scripts/perf/make-media.sh <folder> [films] [albums]
#
# The defaults (1,500 films and 50 albums of 10 songs: 2,000 items) are the ones in
# docs/MINIMUM-HARDWARE.md; CI makes a smaller one. Each clip is over 5 MB, since smaller
# video files are taken for samples and passed over.
set -euo pipefail
root=${1:?a folder for the library}
films=${2:-1500}
albums=${3:-50}
mkdir -p "$root/src" "$root/movies" "$root/music"
cd "$root/src"
enc=(-c:v libx264 -preset veryfast -b:v 3000k -minrate 3000k -maxrate 3000k -bufsize 3000k -pix_fmt yuv420p)
[ -f a.mkv ] || ffmpeg -nostdin -loglevel error -f lavfi -i testsrc2=size=1280x720:rate=24 -f lavfi -i sine=frequency=440 -t 20 "${enc[@]}" -c:a aac -shortest a.mkv
[ -f b.mp4 ] || ffmpeg -nostdin -loglevel error -f lavfi -i testsrc=size=1920x1080:rate=25 -f lavfi -i sine=frequency=330 -vf noise=alls=12:allf=t -t 20 "${enc[@]}" -c:a aac -shortest b.mp4
[ -f c.mkv ] || ffmpeg -nostdin -loglevel error -f lavfi -i smptebars=size=720x480:rate=30 -f lavfi -i sine=frequency=550 -vf noise=alls=12:allf=t -t 20 "${enc[@]}" -c:a ac3 -shortest c.mkv
srcs=(a.mkv b.mp4 c.mkv)
for i in $(seq 1 "$films"); do
  s=${srcs[$((i % 3))]}
  name=$(printf "Test Film %04d (%d)" "$i" $((1950 + i % 70)))
  mkdir -p "$root/movies/$name"
  [ -e "$root/movies/$name/$name.${s##*.}" ] || ln "$root/src/$s" "$root/movies/$name/$name.${s##*.}"
done
for a in $(seq 1 "$albums"); do
  artist=$(printf "Artist %02d" $(((a - 1) / 2 + 1)))
  album=$(printf "Album %03d" "$a")
  d="$root/music/$artist/$album"
  mkdir -p "$d"
  for t in $(seq 1 10); do
    f=$(printf "%s/%02d Song %03d-%02d.mp3" "$d" "$t" "$a" "$t")
    [ -f "$f" ] || ffmpeg -nostdin -loglevel error -f lavfi -i "sine=frequency=$((200 + a * 7 + t)):duration=8" -c:a libmp3lame -b:a 64k \
      -metadata artist="$artist" -metadata album_artist="$artist" -metadata album="$album" -metadata title="Song $a-$t" \
      -metadata track="$t/10" -metadata date=$((1980 + a % 40)) -metadata genre="Genre $((a % 7))" "$f"
  done
done
echo "films: $(find "$root/movies" -type f | wc -l), songs: $(find "$root/music" -type f | wc -l)"
