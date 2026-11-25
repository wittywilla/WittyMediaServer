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
  docker run --rm --entrypoint ffmpeg -v "$media/Movies/$title":/out "$IMG" -v error -y -f lavfi -i "$sync_v" -f lavfi -i "$sync_a" -t 180 $opts "/out/$title.$ext" || { echo "FAIL: could not make $title"; exit 1; }
done
# The same at 24 fps (each flash on a frame) in the shapes real libraries have, for the start test:
# open-GOP H.264 with E-AC-3 5.1 (Blu-ray style), HEVC 10-bit, sound that starts 0.5 s after the
# picture, and HEVC 10-bit HDR10, which is tone-mapped (0.8.5 mapped it through Vulkan in software,
# and HDR films never started on a NAS).
start_v="testsrc2=s=1280x720:r=24,drawbox=x=320:y=180:w=640:h=360:color=black:t=fill,drawbox=x=320:y=180:w=640:h=360:color=white:t=fill:enable='lt(mod(t,2),0.1)'"
x265="-c:v libx265 -preset ultrafast -x265-params log-level=error:keyint=96"
for clip in "Mike (2020)|mkv|-c:v libx264 -preset veryfast -x264-params open-gop=1:keyint=48:min-keyint=48:bluray-compat=1 -pix_fmt yuv420p -ac 6 -c:a eac3 -b:a 384k" \
  "Nova (2020)|mkv|$x265 -pix_fmt yuv420p10le -ac 6 -c:a eac3 -b:a 384k" \
  "Oslo (2020)|mkv|-c:v libx264 -preset veryfast -g 48 -pix_fmt yuv420p -ac 6 -c:a ac3 -b:a 384k -af atrim=start=0.5,asetpts=PTS-STARTPTS+0.5/TB" \
  "Pluto (2020)|mkv|$x265:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:hdr10=1:master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,1):max-cll=1000,400 -pix_fmt yuv420p10le -color_primaries bt2020 -color_trc smpte2084 -colorspace bt2020nc -c:a aac -b:a 160k"; do
  IFS='|' read -r title ext opts <<<"$clip"
  mkdir -p "$media/Movies/$title" && chmod a+rwx "$media/Movies/$title"
  # shellcheck disable=SC2086
  docker run --rm --entrypoint ffmpeg -v "$media/Movies/$title":/out "$IMG" -v error -y -f lavfi -i "$start_v" -f lavfi -i "$sync_a" -t 90 $opts "/out/$title.$ext" || { echo "FAIL: could not make $title"; exit 1; }
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
# Sound and picture still apart back at 1x after 4x (exit 4: more than 40 ms from a fresh start at the same place)
# fails the run. Apart while fast (exit 3) is reported for now: on a one-processor runner the browser's
# pitch-keeping time stretch puts the sound 25-50 ms out (the drift follow-up); AVSYNC_GATE=1 fails that too.
avsync=0
WV_BROWSER=$browser WMS_URL=http://127.0.0.1:$PORT WMS_TOKEN=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['token'])" "$NASPLAY_STATE") \
  node web/scripts/test-avsync.mjs $ids || avsync=$?
if [ "$avsync" = 3 ] && [ "${AVSYNC_GATE:-0}" != 1 ]; then
  echo "::warning title=A/V sync::sound more than 40 ms from the picture while fast (reported, not failing; see the drift follow-up)"
elif [ "$avsync" != 0 ]; then
  exit 1
fi

# Every film starts with its picture and its sound in step: from the start, resumed part way, and after a
# skip, on the open-GOP, HEVC 10-bit, late-sound and HDR10 clips (web/scripts/test-avstart.mjs). Black or
# a held picture over 0.2 s, or sound more than 40 ms from the same clip's start, fails the run.
start_ids=$(python3 - "$NASPLAY_STATE" <<'PY'
import json, sys, urllib.request
st = json.load(open(sys.argv[1]))
r = urllib.request.Request(st['url'] + '/api/items?limit=50&kind=video', headers={'Authorization': 'Bearer ' + st['token']})
items = json.load(urllib.request.urlopen(r))['items']
print(' '.join(str(i['id']) for i in items if i["title"] in ("Mike", "Nova", "Oslo", "Pluto")))
PY
)
[ "$(wc -w <<<"$start_ids")" = 4 ] || { echo "FAIL: the start clips aren't all in the library ($start_ids)"; exit 1; }
echo "== Starts: picture at once, sound in step"
WV_BROWSER=$browser WMS_URL=http://127.0.0.1:$PORT WMS_TOKEN=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['token'])" "$NASPLAY_STATE") \
  AVSTART_STARTS=0,37.3,seek:30 node web/scripts/test-avstart.mjs $start_ids || {
  echo "-- server log (transcode)"; docker logs "$name" 2>&1 | grep -E 'cat=transcode|cat=playback' | grep -v 'level=DEBUG' | tail -30; exit 1; }
