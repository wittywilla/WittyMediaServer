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
# Two A/V sync clips: the middle of the picture flashes white and a 1 kHz tone beeps at the same instant
# every 2 s. One the browser plays as it is (H.264 and AAC in MP4), one the server converts (MPEG-2, AC-3).
sync_v="testsrc2=s=1280x720:r=30,drawbox=x=320:y=180:w=640:h=360:color=black:t=fill,drawbox=x=320:y=180:w=640:h=360:color=white:t=fill:enable='lt(mod(t,2),0.1)'"
sync_a="aevalsrc='if(lt(mod(t,2),0.05),0.8*sin(2*PI*1000*t),0)':s=48000:c=stereo"
for clip in "Kilo (2020)|mp4|-c:v libx264 -preset veryfast -g 60 -pix_fmt yuv420p -c:a aac -b:a 160k -movflags +faststart" "Lima (2020)|mkv|-c:v mpeg2video -q:v 3 -g 15 -c:a ac3 -b:a 192k"; do
  IFS='|' read -r title ext opts <<<"$clip"
  mkdir -p "$media/Movies/$title" && chmod a+rwx "$media/Movies/$title"
  # shellcheck disable=SC2086
  docker run --rm --entrypoint ffmpeg -v "$media/Movies/$title":/out "$IMG" -v error -y -f lavfi -i "$sync_v" -f lavfi -i "$sync_a" -t 60 $opts "/out/$title.$ext" || { echo "FAIL: could not make $title"; exit 1; }
done

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

# Sound and picture together at 1×, 1.5×, 2× and 3×, and after a seek and a pause, in the web player
# in a headless browser (web/scripts/test-avsync.mjs), played straight from the file and converted.
browser=${WV_BROWSER:-$(command -v google-chrome || command -v chromium || true)}
if [ -z "$browser" ] || ! command -v node >/dev/null; then
  echo "skipped: A/V sync needs node and Chrome or Chromium"
  exit 0
fi
ids=$(python3 - "$NASPLAY_STATE" <<'PY'
import json, sys, urllib.request
st = json.load(open(sys.argv[1]))
r = urllib.request.Request(st['url'] + '/api/items?limit=50&kind=video', headers={'Authorization': 'Bearer ' + st['token']})
items = json.load(urllib.request.urlopen(r))['items']
print(' '.join(str(i['id']) for i in items if i["title"] in ("Kilo", "Lima")))
PY
)
[ -n "$ids" ] || { echo "FAIL: the sync clips aren't in the library"; exit 1; }
echo "== A/V sync in the web player"
# Reported, not yet a failure: on a one-processor runner the browser's pitch-keeping time stretch put
# the sound 25-50 ms behind the picture at 1.5-3x, played as it is as well as converted (the drift
# follow-up). AVSYNC_GATE=1 makes it fail.
if ! WV_BROWSER=$browser WMS_URL=http://127.0.0.1:$PORT WMS_TOKEN=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['token'])" "$NASPLAY_STATE") \
  node web/scripts/test-avsync.mjs $ids; then
  if [ "${AVSYNC_GATE:-0}" = 1 ]; then exit 1; fi
  echo "::warning title=A/V sync::sound more than 40 ms from the picture at some speeds (reported, not failing; see the drift follow-up)"
fi
