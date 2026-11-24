#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Starts a smoke-test server and sets it up the way a new owner would: an administrator, the three
# tiny libraries, WittyPlay and WittyTuner from the test catalogue, a Game Boy core and the demo
# tuner. Writes $SMOKE_OUT/server.env (WMS_URL, WMS_NAME, WMS_PASS, WMS_TOKEN) for the next steps.
#
#   scripts/smoke/server.sh <build folder> <media folder> [port]
#
# The server's PID is in $SMOKE_OUT/server.pid; stop it with `kill $(cat $SMOKE_OUT/server.pid)`.
# SMOKE_LISTEN overrides the address it listens on (default 127.0.0.1). SMOKE_CORES is a folder
# kept between runs for downloaded cores (CI caches it).
set -uo pipefail
build=$(cd "${1:?build folder}" && pwd)
media=$(cd "${2:?media folder}" && pwd)
port=${3:-19780}
. "$(dirname "$0")/lib.sh"

data=$SMOKE_OUT/data
mkdir -p "$data"
WMS_URL=http://127.0.0.1:$port
WMS_NAME=smoke
WMS_PASS=${WMS_PASS:-$(head -c 18 /dev/urandom | base64 | tr -d '/+=')}
export WMS_URL WMS_NAME WMS_PASS

step "Start the catalogue and the server"
cport=$((port + 9))
python3 -m http.server --bind 127.0.0.1 "$cport" --directory "$build/catalog" >"$SMOKE_OUT/catalog-http.log" 2>&1 &
echo $! >"$SMOKE_OUT/catalog.pid"
(
  export WMS_NO_BROWSER=1 WMS_GAMING_TESTCORE=1
  export WMS_UPDATE_KEY=$(cat "$build/catalog/test-key.pub") WMS_MODULE_CATALOG=http://127.0.0.1:$cport/catalog.json
  exec "$build/bin/wittymediaserver" serve -data "$data" -listen "${SMOKE_LISTEN:-127.0.0.1}:$port" -open-browser=false </dev/null
) >"$SMOKE_OUT/server.log" 2>&1 &
echo $! >"$SMOKE_OUT/server.pid"
until_ok 60 "the server answers" curl -fsS -m 2 "$WMS_URL/api/health" || exit 1
until_ok 30 "the setup code" test -s "$data/setup-code.txt" || exit 1

step "Create the administrator"
code=$(tr -d '\r\n ' <"$data/setup-code.txt")
r=$(curl -sS -H 'Content-Type: application/json' -d "{\"name\":\"$WMS_NAME\",\"password\":\"$WMS_PASS\",\"setupCode\":\"$code\",\"acknowledgedLegal\":true,\"device\":\"smoke\"}" "$WMS_URL/api/setup")
WMS_TOKEN=$(json "d['token']" <<<"$r" 2>>"$LOG") || { echo "$r" >>"$LOG"; fail "no token from /api/setup"; }
export WMS_TOKEN
api POST /api/setup/complete '{}' >>"$LOG"
# Nothing written beside the media, no online lookups: the test has no internet to depend on.
api PUT /api/settings '{"saveMetadata":"0","saveWittyinfo":"0"}' >>"$LOG"
{ echo "WMS_URL=$WMS_URL"; echo "WMS_NAME=$WMS_NAME"; echo "WMS_PASS=$WMS_PASS"; echo "WMS_TOKEN=$WMS_TOKEN"; } >"$SMOKE_OUT/server.env"

step "Install WittyPlay and WittyTuner"
api POST /api/modules/check >>"$LOG"
for id in wittyplay wittytuner; do api POST "/api/modules/$id/add" '{}' >>"$LOG"; echo >>"$LOG"; done
installed() { api GET /api/modules | json "all(any(m['id'] == i and m.get('installed') for m in d['modules']) for i in ('wittyplay', 'wittytuner')) or exit(1)" >/dev/null; }
until_ok 240 "both modules installed" installed || exit 1

step "Add the libraries"
for l in "Smoke Films|movies|Movies" "Smoke Music|music|Music" "Smoke Games|games|Games"; do
  IFS='|' read -r name type sub <<<"$l"
  id=$(api POST /api/libraries "{\"name\":\"$name\",\"type\":\"$type\",\"paths\":[\"$media/$sub\"]}" | tee -a "$LOG" | json "d['id']")
  echo >>"$LOG"
  api POST "/api/libraries/$id/scan" '{}' >>"$LOG"
  echo >>"$LOG"
done
scanned() { [ "$(api GET /api/libraries | json "sum(1 for l in d if (l.get('itemCount') or 0) > 0)")" -ge 3 ]; }
until_ok 120 "all three libraries hold something" scanned || { api GET /api/libraries >>"$LOG"; exit 1; }

step "Install a Game Boy core"
# The installer downloads it from the libretro build server; SMOKE_CORES keeps a copy between runs.
cores=$data/modules/wittyplay/data/cores
if [ -n "${SMOKE_CORES:-}" ] && [ -s "$SMOKE_CORES/gambatte_libretro.so" ]; then
  mkdir -p "$cores" && cp "$SMOKE_CORES"/gambatte* "$cores/" && echo "gambatte from the cache" >>"$LOG"
else
  api POST /api/v1/modules/wittyplay/gaming/cores/gambatte/install '{}' >>"$LOG"
  until_ok 180 "gambatte installed" test -s "$cores/gambatte_libretro.so" || exit 1
  if [ -n "${SMOKE_CORES:-}" ]; then mkdir -p "$SMOKE_CORES" && cp "$cores"/gambatte* "$SMOKE_CORES/"; fi
fi

step "Add the demo tuner"
api POST /api/v1/modules/wittytuner/tuners '{"driver":"demo","name":"Demo tuner","options":{},"enabled":true}' >>"$LOG"
echo >>"$LOG"
api POST /api/v1/modules/wittytuner/setup/tv '{"skip":false}' >>"$LOG"
tid=$(api GET /api/v1/modules/wittytuner/tuners | json "[t['id'] for t in d if t.get('driver') == 'demo'][0]") || fail "no demo tuner"
channels() { [ "$(api GET /api/v1/modules/wittytuner/tuners | json "[t.get('clear', 0) for t in d if t['id'] == $tid][0]")" -gt 0 ]; }
until_ok 60 "the demo tuner's channels" channels || { api GET /api/v1/modules/wittytuner/tuners >>"$LOG"; exit 1; }
api POST "/api/v1/modules/wittytuner/tuners/$tid/channels/enable" '{"enabled":true}' >>"$LOG"

ok
echo "server ready: $WMS_URL (data $data)"
