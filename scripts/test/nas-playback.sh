#!/bin/bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Plays a conversion in a container held to one processor and 1 GB, as a small NAS would run the
# image, at 2×, with the web app's player and with the Android app's, and fails on any stall after
# the start or an end before the true end. See scripts/nasplay/README.md.
#   scripts/test/nas-playback.sh <image> [cpus] [memory]
set -uo pipefail
IMG=${1:?usage: nas-playback.sh <image> [cpus] [memory]}
CPUS=${2:-1}
PORT=${NASPLAY_PORT:-19811}
MEM=${3:-1g}
cd "$(dirname "$0")/../.." || exit 1
name=wms-nasplay-$$
vol=wms-nasplay-$$
media=$(mktemp -d)
export NASPLAY_STATE=$media/state.json
cleanup() { docker rm -f "$name" >/dev/null 2>&1; docker volume rm "$vol" >/dev/null 2>&1; rm -rf "$media"; }
trap cleanup EXIT

# A clip no browser plays as it is (MPEG-2 in Matroska, AC-3 sound), so the server converts it:
# 90 seconds of 720p, made with the image's own FFmpeg.
mkdir -p "$media/Movies/NAS Test (2024)"
chmod -R a+rwx "$media"
docker run --rm --entrypoint ffmpeg -v "$media/Movies/NAS Test (2024)":/out "$IMG" -v error -y \
  -f lavfi -i testsrc2=size=1280x720:rate=25 -f lavfi -i sine=frequency=440:sample_rate=48000 -t 90 \
  -c:v mpeg2video -q:v 4 -g 12 -c:a ac3 -b:a 192k "/out/NAS Test (2024).mkv" || { echo "FAIL: could not make the clip"; exit 1; }

docker run -d --name "$name" --cpus "$CPUS" --memory "$MEM" --memory-swap "$MEM" \
  --read-only --tmpfs /tmp --cap-drop ALL --security-opt no-new-privileges:true \
  -p 127.0.0.1:$PORT:9797 -v "$vol":/config -v "$media":/media:ro "$IMG" >/dev/null
py=scripts/nasplay/nasplay.py
python3 $py setup --url http://127.0.0.1:$PORT --container "$name" || { docker logs "$name" 2>&1 | tail -40; exit 1; }
fails=0
for client in hlsjs exo-new; do
  echo "== $client at 2x, $CPUS CPU, $MEM"
  if ! python3 $py play --container "$name" --title "NAS Test" --rate 2 --client "$client" --fail-on-stall --timeout 300; then
    fails=$((fails + 1))
  fi
done
if [ $fails -gt 0 ]; then
  echo "-- server log (transcode)"
  docker logs "$name" 2>&1 | grep -E 'cat=transcode|cat=playback' | grep -v 'level=DEBUG' | tail -40
  exit 1
fi
echo "ok: played to the end at 2x with no stall, in both players"
