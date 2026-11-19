#!/bin/bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
#
# Peer-to-peer connections across simulated networks, on one Linux machine
# (or WSL), with network namespaces, iptables NAT and tc netem. Needs root.
#
#   go build -o bin/p2p-netsim ./scripts/p2p-netsim         (GOOS=linux)
#   (in WittyRelay) go build -o bin/wittyrelay ./cmd/wittyrelay
#   sudo scripts/p2p-netsim/netsim.sh bin results [scenario ...]
#
# The "internet" is a bridge in a namespace of its own, with addresses in
# 80.0.0.0/16 that exist only there. On it: a WittyRelay, five ordinary nodes
# (observers: how a node learns its outside address, as the public network
# does for real), and the outside of two home routers. Behind one router is
# the server's node, behind the other the app's. Each scenario sets the
# routers up differently. Nothing here reaches the real internet.
#
# Results: one JSON file per scenario from the app's side, the server's
# status, and every program's log, in the results folder; a summary at the end.
set -uo pipefail

BIN=${1:?folder with p2p-netsim and wittyrelay}
OUT=${2:?folder for results}
shift 2
SCENARIOS=${*:-"cone symmetric-app both-symmetric cgnat-server udp-blocked lossy relay-limit no-relay link-drop address-change many-apps"}
mkdir -p "$OUT"
BIN=$(cd "$BIN" && pwd)
OUT=$(cd "$OUT" && pwd)
KEY=netsim-access-key-0123456789
# libp2p's own account of hole punching, in each program's log.
export GOLOG_LOG_LEVEL=${GOLOG_LOG_LEVEL:-error,p2p-holepunch=debug}
NS="inet relay obs rs srv rc cli cgn"

ns() { ip netns exec "$@"; }

teardown() {
  for n in $NS; do
    ip netns pids "$n" 2>/dev/null | xargs -r kill 2>/dev/null
  done
  sleep 0.5
  for n in $NS; do
    ip netns pids "$n" 2>/dev/null | xargs -r kill -9 2>/dev/null
    ip netns del "$n" 2>/dev/null
  done
  true
}

# link A B ifA ifB: a cable between two namespaces.
link() { ip link add "$3" netns "$1" type veth peer name "$4" netns "$2"; }

# onto_internet NS IF ADDR...: a cable from NS to the internet's bridge.
n_inet=0
onto_internet() {
  local n=$1 dev=$2; shift 2
  n_inet=$((n_inet + 1))
  link "$n" inet "$dev" "p$n_inet"
  ns inet ip link set "p$n_inet" master br0 up
  for a in "$@"; do ns "$n" ip addr add "$a/16" dev "$dev"; done
  ns "$n" ip link set "$dev" up
}

# router NAME WANNS LANIF LANNET MODE: a home router. MODE is cone (one
# outside port per inside port, whoever it talks to: what most routers do),
# or symmetric (a new outside port for every destination).
router() {
  local r=$1 lan_ip=$2 mode=$3
  ns "$r" sysctl -qw net.ipv4.ip_forward=1
  local rand=""
  [ "$mode" = symmetric ] && rand="--random-fully"
  ns "$r" iptables -t nat -A POSTROUTING -o wan -j MASQUERADE $rand
  # Nothing comes in from outside unless it answers something sent out.
  ns "$r" iptables -P FORWARD DROP
  ns "$r" iptables -A FORWARD -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  ns "$r" iptables -A FORWARD -i lan -o wan -j ACCEPT
  # And like a real home router, it ignores what arrives uninvited rather than
  # answering it (Linux would answer a stray connection with a reset, which
  # breaks hole punching where a home router wouldn't).
  ns "$r" iptables -A INPUT -i wan -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  ns "$r" iptables -A INPUT -i wan -j DROP
}

setup() {
  local server_nat=$1 app_nat=$2 extra=$3
  teardown
  n_inet=0
  for n in $NS; do ip netns add "$n"; ns "$n" ip link set lo up; done
  ns inet ip link add br0 type bridge
  ns inet ip link set br0 up
  onto_internet relay eth0 80.0.0.10
  onto_internet obs eth0 80.0.0.21 80.0.0.22 80.0.0.23 80.0.0.24 80.0.0.25

  # The server's side: a home router, and for cgnat-server a provider's shared
  # address in front of it (the router's outside address is 100.64.0.2).
  if [ "$extra" = cgnat-server ]; then
    onto_internet cgn wan 80.0.1.1
    link cgn rs lan wan
    ns cgn ip addr add 100.64.0.1/24 dev lan; ns cgn ip link set lan up
    ns rs ip addr add 100.64.0.2/24 dev wan; ns rs ip link set wan up
    ns rs ip route add default via 100.64.0.1
    router cgn "" cone
  else
    onto_internet rs wan 80.0.1.1
  fi
  link rs srv lan eth0
  ns rs ip addr add 192.168.1.1/24 dev lan; ns rs ip link set lan up
  ns srv ip addr add 192.168.1.10/24 dev eth0; ns srv ip link set eth0 up
  ns srv ip route add default via 192.168.1.1
  router rs 192.168.1.1 "$server_nat"

  # The app's side.
  onto_internet rc wan 80.0.2.1
  link rc cli lan eth0
  ns rc ip addr add 192.168.2.1/24 dev lan; ns rc ip link set lan up
  ns cli ip addr add 192.168.2.10/24 dev eth0; ns cli ip link set eth0 up
  ns cli ip route add default via 192.168.2.1
  router rc 192.168.2.1 "$app_nat"
  case "$extra" in
    udp-blocked) # an office or school network: no UDP out at all
      ns rc iptables -I FORWARD -i lan -p udp -j DROP ;;
    lossy) # a poor mobile connection: 120 ms each way, give or take 30, and 3% of packets lost
      ns rc tc qdisc add dev wan root netem delay 120ms 30ms loss 3% ;;
  esac
}

start_parts() {
  local dir=$1 relay_kbps=$2 with_relay=$3 devices=$4
  ns obs "$BIN/p2p-netsim" observers -ips 80.0.0.21,80.0.0.22,80.0.0.23,80.0.0.24,80.0.0.25 -out "$dir/observers.txt" >"$dir/observers.log" 2>&1 &
  local relay_arg=""
  if [ "$with_relay" = yes ]; then
    mkdir -p "$dir/relaydata"
    ns relay env WITTYRELAY_DATA_DIR="$dir/relaydata" WITTYRELAY_HTTP=80.0.0.10:8080 WITTYRELAY_KEYS=$KEY \
      WITTYRELAY_ANNOUNCE=/ip4/80.0.0.10/udp/4001/quic-v1,/ip4/80.0.0.10/tcp/4001 WITTYRELAY_KBPS="$relay_kbps" \
      WITTYRELAY_LOG_LEVEL=debug "$BIN/wittyrelay" >"$dir/relay.log" 2>&1 &
    for _ in $(seq 1 50); do grep -q '"relay address' "$dir/relay.log" 2>/dev/null && break; sleep 0.2; done
    local rid
    rid=$(grep -o '12D3KooW[1-9A-HJ-NP-Za-km-z]*' "$dir/relay.log" | head -1)
    relay_arg="-relay /ip4/80.0.0.10/udp/4001/quic-v1/p2p/$rid -key $KEY"
  fi
  for _ in $(seq 1 50); do [ -s "$dir/observers.txt" ] && break; sleep 0.2; done
  # shellcheck disable=SC2086
  ns srv "$BIN/p2p-netsim" server $relay_arg -device "$devices" -observers "$dir/observers.txt" -out "$dir/server.json" >"$dir/server.log" 2>&1 &
  # Wait until the server holds a place on the relay (its addresses then
  # include one through it), or 60 s.
  for _ in $(seq 1 120); do
    [ "$with_relay" = no ] && [ -s "$dir/server.json" ] && sleep 20 && break
    grep -q 'p2p-circuit' "$dir/server.json" 2>/dev/null && break
    sleep 0.5
  done
}

run() {
  local name=$1 dir="$OUT/$1"
  rm -rf "$dir"; mkdir -p "$dir"
  local snat=cone anat=cone extra=none kbps=0 relay=yes clients=1 loop="" during=""
  case "$name" in
    cone) ;;
    symmetric-app) anat=symmetric ;;
    both-symmetric) snat=symmetric; anat=symmetric ;;
    cgnat-server) extra=cgnat-server ;;
    udp-blocked) anat=symmetric; extra=udp-blocked ;;
    lossy) extra=lossy ;;
    relay-limit) anat=symmetric; snat=symmetric; kbps=4000 ;;
    no-relay) anat=symmetric; snat=symmetric; relay=no ;;
    link-drop) loop=75s; during=link-drop ;;
    address-change) loop=75s; during=address-change ;;
    many-apps) anat=symmetric; clients=4 ;;
    *) echo "unknown scenario $name"; return ;;
  esac
  echo "== $name (server: $snat, app: $anat, $extra, relay: $relay)"
  setup "$snat" "$anat" "$extra"
  local ids=""
  for i in $(seq 2 $((clients + 1))); do ids="$ids,$("$BIN/p2p-netsim" id -seed "$i")"; done
  start_parts "$dir" "$kbps" "$relay" "${ids#,}"
  local pids=""
  for i in $(seq 2 $((clients + 1))); do
    local args="-server $dir/server.json -observers $dir/observers.txt -mb 8 -seed $i -out $dir/client$i.json"
    [ -n "$loop" ] && args="$args -loop $loop"
    [ "$relay" = no ] && args="$args -wait 60s"
    # shellcheck disable=SC2086
    ns cli "$BIN/p2p-netsim" client $args >"$dir/client$i.log" 2>&1 &
    pids="$pids $!"
  done
  case "$during" in
    link-drop) # the app's connection drops for 12 seconds (a tunnel, a lift)
      for _ in $(seq 1 240); do grep -q '"mbps"' "$dir/client2.json" 2>/dev/null && break; [ -s "$dir/client2.json" ] && break; sleep 0.5; done
      sleep 20; echo "   link down"; ns rc ip link set wan down; sleep 12; ns rc ip link set wan up; echo "   link up" ;;
    address-change) # the app's network gets a new outside address (Wi-Fi to mobile data)
      sleep 45; echo "   new outside address"
      ns rc ip addr del 80.0.2.1/16 dev wan; ns rc ip addr add 80.0.2.99/16 dev wan
      ns rc conntrack -F 2>/dev/null || true ;;
  esac
  # shellcheck disable=SC2086
  wait $pids
  cp "$dir/server.json" "$dir/server-final.json" 2>/dev/null
  teardown
}

trap teardown EXIT
for s in $SCENARIOS; do run "$s"; done

echo
echo "scenario         app  ok  first     now       transport  connect  direct-after  Mbit/s  outage  error"
for s in $SCENARIOS; do
  for f in "$OUT/$s"/client*.json; do
    [ -f "$f" ] || { printf '%-16s  -    (no result)\n' "$s"; continue; }
    python3 - "$s" "$f" <<'PY'
import json, sys, os
s, f = sys.argv[1], sys.argv[2]
r = json.load(open(f))
print("%-16s %-4s %-3s %-9s %-9s %-10s %6.1fs  %12s  %6.1f  %6s  %s" % (
    s, os.path.basename(f)[6:-5], "yes" if r.get("ok") else "NO", r.get("firstPath", ""), r.get("path", ""), r.get("transport", "") or "-",
    r.get("connectSec", 0), ("%.1fs" % r["directAfterSec"]) if r.get("directAfterSec") else "-", r.get("mbps", 0),
    ("%dms" % r["longestOutageMs"]) if r.get("longestOutageMs") else "-", (r.get("error") or "")[:70]))
PY
  done
done
