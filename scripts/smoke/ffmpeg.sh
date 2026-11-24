#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# FFmpeg for the smoke tests, without waiting on Ubuntu's mirrors (a stalled apt-get once held a job
# until its time limit): the static Linux build scripts/ffmpeg.lock names, checked against its
# SHA-256 and kept in <folder> (CI caches it). If that download fails too, apt-get, with a time limit.
#
#   scripts/smoke/ffmpeg.sh <folder>      puts ffmpeg and ffprobe in <folder>/bin and prints that path
#
# In GitHub Actions the folder is also added to PATH for the steps that follow.
set -uo pipefail
dir=${1:?where to keep FFmpeg}
lock=$(dirname "$0")/../ffmpeg.lock
mkdir -p "$dir/bin"
dir=$(cd "$dir" && pwd)
read -r want url < <(awk '$1 == "linux64" { print $2, $3 }' "$lock")
file=$dir/$(basename "$url")

have() { [ -x "$dir/bin/ffmpeg" ] && [ -x "$dir/bin/ffprobe" ] && "$dir/bin/ffmpeg" -hide_banner -version >/dev/null 2>&1; }
fetched() { [ -s "$file" ] && [ "$(sha256sum "$file" | cut -d' ' -f1)" = "$want" ]; }

if ! have; then
  for try in 1 2 3; do
    fetched && break
    rm -f "$file"
    curl -fsSL --retry 3 --connect-timeout 20 --max-time 300 -o "$file.part" "$url" && mv "$file.part" "$file"
    fetched || echo "FFmpeg download $try: not the file scripts/ffmpeg.lock names" >&2
  done
  if fetched; then
    tar -xJf "$file" -C "$dir" --wildcards '*/bin/ffmpeg' '*/bin/ffprobe' &&
      find "$dir" -path '*/bin/ffmpeg' -not -path "$dir/bin/*" -exec cp {} "$dir/bin/" \; &&
      find "$dir" -path '*/bin/ffprobe' -not -path "$dir/bin/*" -exec cp {} "$dir/bin/" \;
    # Only the two programs are kept (and cached).
    rm -rf "$file" "$dir"/ffmpeg-*/
  fi
fi
if ! have; then
  echo "The static FFmpeg isn't here; trying apt-get, for 5 minutes at most" >&2
  sudo rm -f /var/lib/man-db/auto-update
  timeout 300 sudo apt-get -o Acquire::Retries=3 -o Acquire::http::Timeout=30 update -qq &&
    timeout 300 sudo apt-get -o Acquire::Retries=3 -o Acquire::http::Timeout=30 install -y -qq ffmpeg >/dev/null &&
    ln -sf "$(command -v ffmpeg)" "$dir/bin/ffmpeg" && ln -sf "$(command -v ffprobe)" "$dir/bin/ffprobe"
fi
have || { echo "no FFmpeg" >&2; exit 1; }
"$dir/bin/ffmpeg" -hide_banner -version | head -1 >&2
[ -n "${GITHUB_PATH:-}" ] && echo "$dir/bin" >>"$GITHUB_PATH"
echo "$dir/bin"
