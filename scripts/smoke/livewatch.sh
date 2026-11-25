#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Live TV for ten minutes on a small NAS behind a home upload: the server held to 2 CPUs and 2 GB, in its own
# network with a 5 Mbit/s, 50 ms link to the browser, and a channel made from a 1080p clip. The web app watches it
# at 720p and 3 Mbit/s (what live TV's Auto quality asks for peer to peer, web/src/pages/tv/liveAuto.ts), and
# scripts/smoke/livewatch.mjs counts the jumps of its clock and its freezes. 0.8.6 froze 49 times in 5 minutes
# here; it must be none.
#
#   sudo -E scripts/smoke/livewatch.sh <build folder> [seconds]     (needs root: a network namespace, tc, a CPU cap)
set -uo pipefail
build=$(cd "${1:?build folder}" && pwd)
secs=${2:-600}
here=$(cd "$(dirname "$0")/../.." && pwd)
. "$(dirname "$0")/lib.sh"
[ "$(id -u)" = 0 ] || { echo "needs root (a network namespace)"; exit 2; }
RUNAS=${SUDO_USER:-root}
HTTP=19792
N=lw-srv
ns() { ip netns exec "$@"; }
teardown() {
  ip netns pids "$N" 2>/dev/null | xargs -r kill 2>/dev/null; sleep 0.5
  ip netns pids "$N" 2>/dev/null | xargs -r kill -9 2>/dev/null
  ip link del lw0 2>/dev/null; ip netns del "$N" 2>/dev/null; true
}
trap 'teardown; chmod -R a+rX "$SMOKE_OUT" 2>/dev/null' EXIT

step "A small NAS behind a 5 Mbit/s upload"
teardown
{
  ip netns add "$N" && ns "$N" ip link set lo up
  ip link add lw0 type veth peer name eth0 netns "$N"
  ip addr add 10.77.0.1/24 dev lw0 && ip link set lw0 up
  ns "$N" ip addr add 10.77.0.2/24 dev eth0 && ns "$N" ip link set eth0 up
  # The server's upload, both ways' delay on its side: 5 Mbit/s and 50 ms there and back.
  ns "$N" tc qdisc add dev eth0 root netem rate 5mbit delay 50ms limit 10000
} >>"$LOG" 2>&1 || fail "the network could not be made"
mkdir -p "$SMOKE_OUT/lw-data" "$SMOKE_OUT/lw-tv"; chown "$RUNAS" "$SMOKE_OUT" "$SMOKE_OUT/lw-data"
# 2 CPUs and 2 GB, as a small NAS's container is given.
systemd-run --scope -q -p CPUQuota=200% -p MemoryMax=2G ip netns exec "$N" sudo -u "$RUNAS" env WMS_NO_BROWSER=1 WMS_FFMPEG="$(command -v ffmpeg)" \
  "$build/bin/wittymediaserver" serve -data "$SMOKE_OUT/lw-data" -listen "0.0.0.0:$HTTP" -open-browser=false </dev/null >"$SMOKE_OUT/lw-server.log" 2>&1 &
S=http://10.77.0.2:$HTTP
sapi() { curl -sS -m 30 -X "$1" -H "Authorization: Bearer ${TOK:-}" -H 'Content-Type: application/json' ${3:+--data "$3"} "$S$2"; }
until_ok 60 "the server answers" curl -fsS -m 2 "$S/api/health" || exit 1
until_ok 30 "the setup code" test -s "$SMOKE_OUT/lw-data/setup-code.txt" || exit 1
PW=$(head -c 18 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')x9
TOK=$(sapi POST /api/setup "{\"name\":\"owner\",\"password\":\"$PW\",\"setupCode\":\"$(tr -d '\r\n ' <"$SMOKE_OUT/lw-data/setup-code.txt")\",\"acknowledgedLegal\":true}" | json "d['token']") \
  || fail "no administrator"
sapi POST /api/setup/complete '{}' >>"$LOG"

step "A channel from a 1080p clip"
ffmpeg -hide_banner -loglevel error -y -f lavfi -i testsrc2=size=1920x1080:rate=25 -f lavfi -i sine=frequency=440:sample_rate=48000 -t 180 \
  -c:v libx264 -preset ultrafast -b:v 8M -maxrate 8M -bufsize 4M -pix_fmt yuv420p -g 50 -c:a aac -shortest "$SMOKE_OUT/lw-tv/Smoke clip (2026).mp4" \
  || fail "making the channel's clip"
chown -R "$RUNAS" "$SMOKE_OUT/lw-tv"
lib=$(sapi POST /api/libraries "{\"name\":\"Smoke TV\",\"type\":\"movies\",\"paths\":[\"$SMOKE_OUT/lw-tv\"]}" | json "d['id']") || fail "the TV library"
scanned() { sapi GET "/api/items?library=$lib&limit=5" | grep -q '"id"'; }
until_ok 90 "the clip is in the library" scanned || exit 1
item=$(sapi GET "/api/items?library=$lib&limit=5" | json "(d.get('items') if isinstance(d, dict) else d)[0]['id']")
ch=$(sapi POST /api/channels "{\"name\":\"Smoke TV\",\"number\":\"9\",\"definition\":{\"mode\":\"simple\",\"rules\":{\"items\":[$item]},\"alignMin\":0,\"filler\":{}}}" | json "d['id']") \
  || fail "the channel"

step "Ten minutes of the channel: no jumps, no freezes"
sudo -u "$RUNAS" env "PATH=$PATH" WMS_URL="$S" WMS_TOKEN="$TOK" SMOKE_TMP="${SMOKE_TMP:-}" WV_BROWSER="${WV_BROWSER:-}" \
  node "$here/scripts/smoke/livewatch.mjs" "$SMOKE_OUT/livewatch" "/tv/watch/lib-$ch" "$secs" h720-b3000 >"$SMOKE_OUT/livewatch.txt" 2>&1
cat "$SMOKE_OUT/livewatch.txt" >>"$LOG"
grep -q '^OK' "$SMOKE_OUT/livewatch.txt" || fail "live TV on a small NAS at 5 Mbit/s: $(grep -m1 '^FAIL' "$SMOKE_OUT/livewatch.txt" | cut -c1-300)"
head -1 "$SMOKE_OUT/livewatch.txt"
finish
