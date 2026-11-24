#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# The pairing lab: pairing a phone with a server from away, across simulated networks, phone
# states and server states, on the first try, with every step on both sides timed. README.md
# says what it covers and how to read the results.
#
#   scripts/pairing-lab/lab.sh build <build folder> [server checkout] [WittyRelay checkout]
#   scripts/pairing-lab/lab.sh list  <set>                         the cases in a set
#   sudo -E scripts/pairing-lab/lab.sh run <build folder> <results folder> <set or case>...
#
# Sets: fast (every pull request), full (nightly, before a release tag), or a filter on the full
# matrix such as net=symmetric-relay, way=link, phone=upgrade (several are ANDed). PL_SHARD=2/8
# runs every eighth case starting with the second. PL_CODE=0 leaves out joining by a code (it
# needs the real internet, for the public address book).
set -uo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
NETS="direct fullcone cone-relay ipv4-only symmetric-relay cgnat-relay udp-blocked ipv6-only docker-home docker-away"
SERVERS="fresh settled restart"
ONLYS="on off"
PHONES="fresh upgrade previous"
WAYS="link code"

# One case per line. The fast set is a cross-section: every network once, every server and phone
# state at least once, both ways in.
FAST="cone-relay.fresh.on.fresh.link
symmetric-relay.fresh.on.fresh.link
symmetric-relay.restart.on.fresh.link
cgnat-relay.settled.on.upgrade.link
udp-blocked.settled.on.fresh.link
ipv6-only.settled.on.fresh.link
cone-relay.settled.off.previous.link
direct.fresh.on.fresh.link
fullcone.settled.on.fresh.link
ipv4-only.settled.off.fresh.link
cone-relay.settled.on.fresh.code
docker-home.settled.on.fresh.link
docker-away.fresh.on.fresh.link"

full() {
  local n s o p w
  for n in $NETS; do for s in $SERVERS; do for o in $ONLYS; do for p in $PHONES; do for w in $WAYS; do
    echo "$n.$s.$o.$p.$w"
  done; done; done; done; done
}

# cases <set or filter or case>...
cases() {
  local a list="" filt=""
  for a in "$@"; do
    case "$a" in
      fast) list="$list"$'\n'"$FAST" ;;
      full) list="$list"$'\n'"$(full)" ;;
      *=*) filt="$filt $a" ;;
      *.*.*.*.*) list="$list"$'\n'"$a" ;;
      *) echo "unknown set $a" >&2; return 2 ;;
    esac
  done
  [ -z "$list" ] && list=$(full)
  local c f k v keep
  while read -r c; do
    [ -z "$c" ] && continue
    IFS=. read -r n s o p w <<<"$c"
    keep=1
    for f in $filt; do
      k=${f%%=*}; v=${f#*=}
      case "$k" in
        net) [[ ",$v," == *",$n,"* ]] || keep=0 ;;
        server) [[ ",$v," == *",$s,"* ]] || keep=0 ;;
        only) [[ ",$v," == *",$o,"* ]] || keep=0 ;;
        phone) [[ ",$v," == *",$p,"* ]] || keep=0 ;;
        way) [[ ",$v," == *",$w,"* ]] || keep=0 ;;
      esac
    done
    [ "${PL_CODE:-1}" = 0 ] && [ "$w" = code ] && keep=0
    # Joining by a code from an IPv6-only network needs the real internet over IPv6.
    [ "$n" = ipv6-only ] && [ "$w" = code ] && [ "${PL_V6_UPLINK:-0}" != 1 ] && keep=0
    [ "$keep" = 1 ] && echo "$c"
  done <<<"$list" | awk '!seen[$0]++' | shard
}

shard() {
  local i=1 k n
  if [ -z "${PL_SHARD:-}" ]; then cat; return; fi
  k=${PL_SHARD%/*}; n=${PL_SHARD#*/}
  while read -r c; do
    [ $(((i - k) % n)) = 0 ] && echo "$c"
    i=$((i + 1))
  done
}

cmd=${1:-}
shift || true
case "$cmd" in
  build)
    out=${1:?build folder}; srv=${2:-$HERE/../..}; relay=${3:-}
    mkdir -p "$out/bin"; out=$(cd "$out" && pwd); srv=$(cd "$srv" && pwd)
    if [ -z "$relay" ]; then
      relay=$out/WittyRelay
      [ -d "$relay/.git" ] || git clone -q --depth 1 ${PL_RELAY_REF:+--branch "$PL_RELAY_REF"} https://github.com/wittywilla/WittyRelay "$relay"
    fi
    set -e
    # The server needs its web app embedded only as files; an empty page will do for the lab.
    if [ ! -f "$srv/web/dist/index.html" ]; then
      mkdir -p "$srv/web/dist"; echo '<!doctype html><title>lab</title>' >"$srv/web/dist/index.html"
    fi
    (cd "$srv" && go build -o "$out/bin/wittymediaserver" ./cmd/wittymediaserver)
    # The phone is built from this lab's own folder against that checkout's library: the one the app embeds.
    rm -rf "$srv/.pl-labphone"; mkdir -p "$srv/.pl-labphone"; cp "$HERE"/labphone/*.go "$srv/.pl-labphone/"
    (cd "$srv" && go build -o "$out/bin/labphone" ./.pl-labphone); rm -rf "$srv/.pl-labphone"
    (cd "$relay" && GOWORK=off go build -o "$out/bin/wittyrelay" ./cmd/wittyrelay)
    { echo "server $(git -C "$srv" describe --tags --always 2>/dev/null)"; echo "relay $(git -C "$relay" describe --tags --always 2>/dev/null)"; } | tee "$out/versions.txt"
    ;;
  list) cases "$@" ;;
  run)
    build=${1:?build folder}; res=${2:?results folder}; shift 2
    [ "$(id -u)" = 0 ] || { echo "needs root (network namespaces): sudo -E $0 run ..."; exit 2; }
    # One run at a time on a machine: cases share the pl-* namespace names.
    exec 9>/run/pairing-lab.lock
    flock -n 9 || { echo "another pairing-lab run is going on this machine (/run/pairing-lab.lock)"; exit 2; }
    mkdir -p "$res"
    cp "$build/versions.txt" "$res/" 2>/dev/null
    mapfile -t todo < <(cases "$@")
    echo "${#todo[@]} cases"
    i=0
    for c in "${todo[@]}"; do
      i=$((i + 1))
      t=$(date +%s)
      timeout --kill-after=10 "${PL_CASE_TIMEOUT:-300}" bash "$HERE/case.sh" "$build" "$res" "$c" >"$res/.case.log" 2>&1
      rc=$?
      [ -f "$res/$c/result.json" ] || { mkdir -p "$res/$c"; python3 "$HERE/report.py" case "$res/$c" "${rc/124/5}"; }
      mv "$res/.case.log" "$res/$c/case.log" 2>/dev/null
      printf '[%d/%d] %-45s %s %ss\n' "$i" "${#todo[@]}" "$c" "$([ $rc = 0 ] && echo pass || echo "FAIL($rc)")" "$(($(date +%s) - t))"
      [ $rc != 0 ] && python3 "$HERE/report.py" timeline "$res/$c" | tail -12 | sed 's/^/      /'
    done
    # Anything a killed case left behind.
    . "$HERE/net.sh"; lab_net_down
    python3 "$HERE/report.py" summary "$res"
    ;;
  *) sed -n 4,16p "$0"; exit 2 ;;
esac
