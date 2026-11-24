#!/bin/bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Plays games in the image the way a NAS runs it: locked down, on Docker's own network, with no
# graphics card. WittyPlay comes from a test catalogue (scripts/make-test-catalog.sh), so the image
# has to be built with GOTAGS=updatetest. Two real cores are downloaded and each plays a tiny ROM
# this script writes itself; each has to send pictures over the server's own port.
#   scripts/test/docker-games.sh <image> <catalogue folder>
# Set KEEP=1 to leave the container running afterwards (port in the output).
set -uo pipefail
IMG=${1:?usage: docker-games.sh <image> <catalogue folder>}
CAT=$(cd "${2:?usage: docker-games.sh <image> <catalogue folder>}" && pwd)
cd "$(dirname "$0")/../.." || exit 1
fails=0
ok() { echo "ok:   $*"; }
bad() { echo "FAIL: $*"; fails=$((fails + 1)); }
name=wms-games-$$
vol=wms-games-$$
games=$(mktemp -d)
cleanup() {
  [ "${KEEP:-}" = 1 ] && return
  docker rm -f "$name" "$name-cat" >/dev/null 2>&1; docker volume rm "$vol" >/dev/null 2>&1; rm -rf "$games"
}
trap cleanup EXIT

# Two ROMs that only loop, written here: an NES one (mapper 0) and a Master System one. A core
# that loads either and runs it sends black pictures, which is all this needs to see.
mkdir -p "$games/NES" "$games/Master System"
python3 - "$games" <<'PY'
import sys
d = sys.argv[1]
prg = bytearray(16384)
prg[0:5] = bytes([0x78, 0xD8, 0x4C, 0x02, 0xC0])  # SEI; CLD; JMP $C002
prg[0x3FFA:0x4000] = bytes([0x00, 0xC0, 0x00, 0xC0, 0x00, 0xC0])  # NMI, reset, IRQ at $C000
open(d + "/NES/Loop Test.nes", "wb").write(b"NES\x1a\x01\x01" + bytes(10) + prg + bytes(8192))
sms = bytearray(32768)
sms[0:3] = bytes([0xF3, 0x18, 0xFE])  # DI; JR -2
open(d + "/Master System/Loop Test.sms", "wb").write(sms)
PY
chmod -R a+rX "$games"

hard=(--read-only --tmpfs /tmp --cap-drop ALL --security-opt no-new-privileges:true)
# DOCKER_ARGS: more for docker run, such as --cpus=1 --memory=1g for a small NAS.
# shellcheck disable=SC2086
docker run -d ${DOCKER_ARGS:-} --name "$name" "${hard[@]}" -p 127.0.0.1::9797 -v "$vol":/config -v "$games":/media/games:ro \
  -e WMS_UPDATE_KEY="$(cat "$CAT/test-key.pub")" -e WMS_MODULE_CATALOG=http://127.0.0.1:8099/catalog.json "$IMG" >/dev/null || { bad "it didn't start"; exit 1; }
# The catalogue, served on the container's own loopback address, the only one a test build reads from.
docker pull -q busybox:1.37 >/dev/null
docker run -d --name "$name-cat" --network "container:$name" -v "$CAT":/c:ro busybox:1.37 httpd -f -p 127.0.0.1:8099 -h /c >/dev/null
for _ in $(seq 1 90); do
  [ "$(docker inspect -f '{{.State.Health.Status}}' "$name" 2>/dev/null)" = healthy ] && break
  sleep 1
done
port=$(docker port "$name" 9797/tcp | head -n1 | sed 's/.*://')
api=http://127.0.0.1:$port/api
code=$(docker logs "$name" 2>&1 | grep -o -E '[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}' | head -1)
pw=$(head -c 18 /dev/urandom | base64 | tr -d '/+=')
tok=$(curl -s -H 'Content-Type: application/json' -d "{\"name\":\"tester\",\"password\":\"$pw\",\"setupCode\":\"$code\",\"acknowledgedLegal\":true}" "$api/setup" |
  python3 -c 'import json,sys;print(json.load(sys.stdin).get("token",""))')
[ -n "$tok" ] || { bad "setup"; docker logs "$name" 2>&1 | tail -20; exit 1; }
H="Authorization: Bearer $tok"
curl -s -X POST -H "$H" "$api/setup/complete" -o /dev/null
curl -s -X POST -H "$H" "$api/modulestore/check" -o /dev/null
curl -s -X POST -H "$H" -H 'Content-Type: application/json' -d '{"id":"wittyplay","confirm":true}' "$api/modulestore/install" -o /dev/null
st=""
for _ in $(seq 1 120); do st=$(curl -s -H "$H" "$api/modulestore/run"); grep -q '"finished"' <<<"$st" && break; sleep 2; done
if grep -q '"state":"done"' <<<"$st"; then ok "WittyPlay installed"; else bad "WittyPlay install: $st"; docker logs "$name" 2>&1 | tail -20; exit 1; fi

P=$api/modules/wittyplay
for core in fceumm gearsystem; do
  curl -s -X POST -H "$H" "$P/gaming/cores/$core/install" -o /dev/null -w '' || true
  for _ in $(seq 1 90); do
    curl -s -H "$H" "$P/gaming/cores/$core/install" | grep -q '"installing":true' || break
    sleep 2
  done
  if docker exec "$name" test -f "/config/modules/wittyplay/data/cores/${core}_libretro.so"; then ok "core $core downloaded for $(docker exec "$name" uname -m)"; else bad "core $core wasn't downloaded"; fi
done

lib=$(curl -s -H "$H" -H 'Content-Type: application/json' -d '{"name":"Games","type":"games","paths":["/media/games"]}' "$api/libraries" |
  python3 -c 'import json,sys;print(json.load(sys.stdin).get("id",""))')
ids=""
for _ in $(seq 1 60); do
  ids=$(curl -s -H "$H" "$api/items?library=$lib&limit=20" | python3 -c '
import json,sys
d=json.load(sys.stdin).get("items") or []
print(" ".join("%s:%s" % (i["id"], i["path"].rsplit(".",1)[-1]) for i in d))')
  [ "$(wc -w <<<"$ids")" -ge 2 ] && break
  sleep 2
done
[ "$(wc -w <<<"$ids")" -ge 2 ] && ok "both games found" || bad "games found: $ids"

for pair in $ids; do
  id=${pair%%:*}; ext=${pair##*:}
  t0=$(date +%s%3N)
  r=$(curl -s -m 120 -H "$H" -H 'Content-Type: application/json' -d '{"client":"browser"}' "$P/games/$id/play")
  started=$(($(date +%s%3N) - t0))
  sid=$(python3 -c 'import json,sys;print(json.load(sys.stdin).get("id",""))' <<<"$r" 2>/dev/null)
  if [ -z "$sid" ]; then bad "$ext: it didn't start: $r"; continue; fi
  modes=$(python3 -c 'import json,sys;print(",".join(m["id"] for m in json.load(sys.stdin).get("modes",[])))' <<<"$r")
  # On Docker's own network WebRTC can't get through, so it mustn't be offered first.
  case $modes in webrtc*) bad "$ext: WebRTC offered behind Docker's network ($modes)";; *) ok "$ext: started in $started ms, modes $modes";; esac
  for kind in h264 jpeg; do
    got=$(node scripts/test/stream-frames.mjs "ws://127.0.0.1:$port/api/modules/wittyplay/play/$sid/stream?token=$tok" "$kind" 8000)
    n=$(sed -n 's/^pictures \([0-9]*\).*/\1/p' <<<"$got")
    if [ "${n:-0}" -ge 10 ]; then ok "$ext: $got over the WebSocket ($kind)"; else bad "$ext ($kind): $got"; fi
  done
  curl -s -X DELETE -H "$H" "$P/play/$sid" -o /dev/null
done

if [ $fails -gt 0 ]; then docker logs "$name" 2>&1 | grep -E 'level=(WARN|ERROR)|cat=games' | cut -c1-400 | tail -30; fi
[ "${KEEP:-}" = 1 ] && echo "kept: $name on port $port"
echo "$fails failed"
[ $fails = 0 ]
