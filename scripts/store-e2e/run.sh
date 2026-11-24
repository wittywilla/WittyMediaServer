#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# The module store's end-to-end test: one fresh test server, the small catalogue from catalog.sh
# served by the test itself, and web/scripts/test-module-store.mjs driving a headless browser.
#
#   scripts/store-e2e/run.sh <server binary built with -tags updatetest> <catalog.sh's output> <out folder> [ffmpeg folder]
#
# STORE_PORT (default 19790) and STORE_CATALOG_PORT (default 8150) choose the ports. Everything it
# starts is stopped when it ends; the server's data and log stay in <out folder>/data.
set -euo pipefail
bin=${1:?the server binary}
cat=${2:?the catalogues}
out=${3:?where to write}
ff=${4:-}
here=$(cd "$(dirname "$0")/../.." && pwd)
port=${STORE_PORT:-19790}
cport=${STORE_CATALOG_PORT:-8150}
mkdir -p "$out"
out=$(cd "$out" && pwd)
cat=$(cd "$cat" && pwd)
data="$out/data"
rm -rf "$data"; mkdir -p "$data"
url="http://127.0.0.1:$port"

(
  export WMS_NO_BROWSER=1 WMS_UPDATE_KEY=$(cat "$cat/a/test-key.pub") WMS_MODULE_CATALOG="http://127.0.0.1:$cport/catalog.json"
  if [ -n "$ff" ]; then export WMS_FFMPEG="$ff/ffmpeg" WMS_FFPROBE="$ff/ffprobe"; fi
  exec "$bin" serve -data "$data" -listen "127.0.0.1:$port" -open-browser=false </dev/null
) >"$out/server.log" 2>&1 &
pid=$!
trap 'kill $pid 2>/dev/null || true; wait $pid 2>/dev/null || true' EXIT

for _ in $(seq 60); do curl -fsS -m 2 "$url/api/health" >/dev/null 2>&1 && break; sleep 1; done
for _ in $(seq 30); do [ -s "$data/setup-code.txt" ] && break; sleep 1; done
code=$(tr -d '\r\n ' <"$data/setup-code.txt")
pass=$(head -c 18 /dev/urandom | base64 | tr -dc A-Za-z0-9)
token=$(curl -sS -H 'Content-Type: application/json' -d "{\"name\":\"store\",\"password\":\"$pass\",\"setupCode\":\"$code\",\"acknowledgedLegal\":true,\"device\":\"store-e2e\"}" "$url/api/setup" |
  node -e 'let s="";process.stdin.on("data",(d)=>s+=d).on("end",()=>console.log(JSON.parse(s).token))')
curl -sS -X POST -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d '{}' "$url/api/setup/complete" >/dev/null
printf '%s' "$token" >"$out/token"

cd "$here/web"
WMS_SERVER="$url" WMS_TOKEN_FILE="$out/token" WMS_CATALOG_DIR="$cat" WMS_CATALOG_PORT="$cport" node scripts/test-module-store.mjs "$out/shots"
