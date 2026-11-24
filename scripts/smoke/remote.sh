#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Remote access across two networks, with nothing on the real internet: the real server behind one
# home router, an app's connector (the real Wittyconnect, as the desktop app runs it) behind
# another, and a WittyRelay on a pretend internet between them. Both routers change ports for every
# connection (symmetric NAT), so the only way through is the relay, as from a phone on mobile data.
#
# The server is set up the way docs/REMOTE.md says: peer to peer on, "Your own WittyRelay" with
# its key, then Connect a device makes an invite link. The connector is given only that link, puts
# itself on the server's device list with it, and the server's API answers through it.
#
#   sudo -E scripts/smoke/remote.sh <build folder>        (needs root: network namespaces)
#
# The build folder is scripts/smoke/build.sh's (wittymediaserver, wittyconnect, wittyrelay), plus
# p2p-netsim, which it builds here if missing. Logs in $SMOKE_OUT.
set -uo pipefail
build=$(cd "${1:?build folder}" && pwd)
here=$(cd "$(dirname "$0")/../.." && pwd)
. "$(dirname "$0")/lib.sh"
[ "$(id -u)" = 0 ] || { echo "needs root (network namespaces)"; exit 2; }
RUNAS=${SUDO_USER:-root}
HTTP=19790
P2P=19791
KEY=smoke-relay-key-$(head -c 9 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')
NS="sm-inet sm-relay sm-obs sm-rs sm-srv sm-rc sm-cli"
ns() { ip netns exec "$@"; }
asuser() { local n=$1; shift; ip netns exec "$n" sudo -u "$RUNAS" env HOME="$(getent passwd "$RUNAS" | cut -d: -f6)" "$@"; }

teardown() {
  for n in $NS; do ip netns pids "$n" 2>/dev/null | xargs -r kill 2>/dev/null; done
  sleep 0.5
  for n in $NS; do ip netns pids "$n" 2>/dev/null | xargs -r kill -9 2>/dev/null; ip netns del "$n" 2>/dev/null; done
  true
}
trap 'teardown; chmod -R a+rX "$SMOKE_OUT" 2>/dev/null' EXIT

step "Two homes and an internet between them"
teardown
{
  [ -x "$build/bin/p2p-netsim" ] || (cd "$here" && go build -o "$build/bin/p2p-netsim" ./scripts/p2p-netsim)
  for n in $NS; do ip netns add "$n"; ip -n "$n" link set lo up; done
  ns sm-inet ip link add br0 type bridge; ns sm-inet ip link set br0 up
  i=0
  wire() { # wire NS DEV ADDR...: a cable from NS to the internet's bridge
    local n=$1 dev=$2; shift 2; i=$((i + 1))
    ip link add "$dev" netns "$n" type veth peer name "p$i" netns sm-inet
    ns sm-inet ip link set "p$i" master br0 up
    for a in "$@"; do ns "$n" ip addr add "$a/16" dev "$dev"; done
    ns "$n" ip link set "$dev" up
  }
  wire sm-relay eth0 80.0.0.10
  wire sm-obs eth0 80.0.0.21 80.0.0.22 80.0.0.23 80.0.0.24 80.0.0.25
  wire sm-rs wan 80.0.0.2
  wire sm-rc wan 80.0.0.3
  home() { # home ROUTER HOST NET: a symmetric NAT router with one computer behind it
    local r=$1 h=$2 net=$3
    ip link add lan netns "$r" type veth peer name eth0 netns "$h"
    ns "$r" ip addr add "$net.1/24" dev lan; ns "$r" ip link set lan up
    ns "$h" ip addr add "$net.10/24" dev eth0; ns "$h" ip link set eth0 up
    ns "$h" ip route add default via "$net.1"
    ns "$r" sysctl -qw net.ipv4.ip_forward=1
    ns "$r" iptables -t nat -A POSTROUTING -o wan -j MASQUERADE --random-fully
    ns "$r" iptables -P FORWARD DROP
    ns "$r" iptables -A FORWARD -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
    ns "$r" iptables -A FORWARD -i lan -o wan -j ACCEPT
    ns "$r" iptables -A INPUT -i wan -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
    ns "$r" iptables -A INPUT -i wan -j DROP
  }
  home sm-rs sm-srv 192.168.1
  home sm-rc sm-cli 192.168.2
  # The phone visiting the server's home first: a second cable, straight to the server, unplugged later.
  ip link add eth1 netns sm-cli type veth peer name eth1 netns sm-srv
  ns sm-srv ip addr add 192.168.3.1/24 dev eth1; ns sm-srv ip link set eth1 up
  ns sm-cli ip addr add 192.168.3.2/24 dev eth1; ns sm-cli ip link set eth1 up
} >>"$LOG" 2>&1 || fail "the networks could not be made"

step "The relay and the internet's ordinary nodes"
mkdir -p "$SMOKE_OUT/relay"
ns sm-relay env WITTYRELAY_DATA_DIR="$SMOKE_OUT/relay" WITTYRELAY_HTTP=80.0.0.10:8080 WITTYRELAY_KEYS="$KEY" \
  WITTYRELAY_ANNOUNCE=/ip4/80.0.0.10/udp/4001/quic-v1,/ip4/80.0.0.10/tcp/4001 WITTYRELAY_LOG_LEVEL=debug \
  "$build/bin/wittyrelay" >"$SMOKE_OUT/relay.log" 2>&1 &
ns sm-obs "$build/bin/p2p-netsim" observers -ips 80.0.0.21,80.0.0.22,80.0.0.23,80.0.0.24,80.0.0.25 -out "$SMOKE_OUT/observers.txt" >"$SMOKE_OUT/observers.log" 2>&1 &
until_ok 15 "the relay's address" grep -q '"relay address' "$SMOKE_OUT/relay.log" || exit 1
RID=$(grep -o '12D3KooW[1-9A-HJ-NP-Za-km-z]*' "$SMOKE_OUT/relay.log" | head -1)
RELAY=/ip4/80.0.0.10/udp/4001/quic-v1/p2p/$RID
echo "relay: $RELAY" >>"$LOG"

step "The server, at home"
mkdir -p "$SMOKE_OUT/data"; chown "$RUNAS" "$SMOKE_OUT" "$SMOKE_OUT/data"
# sudo -u resets PATH: FFmpeg is named, wherever scripts/smoke/ffmpeg.sh or the system put it.
asuser sm-srv env WMS_NO_BROWSER=1 WMS_FFMPEG="$(command -v ffmpeg)" "$build/bin/wittymediaserver" serve -data "$SMOKE_OUT/data" -listen "0.0.0.0:$HTTP" -open-browser=false \
  </dev/null >"$SMOKE_OUT/server.log" 2>&1 &
S=http://127.0.0.1:$HTTP
sapi() { ns sm-srv curl -sS -m 30 -X "$1" -H "Authorization: Bearer ${TOK:-}" -H 'Content-Type: application/json' ${3:+--data "$3"} "$S$2"; }
until_ok 60 "the server answers" ns sm-srv curl -fsS -m 2 "$S/api/health" || exit 1
until_ok 30 "the setup code" test -s "$SMOKE_OUT/data/setup-code.txt" || exit 1
PW=$(head -c 18 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')x9
TOK=$(sapi POST /api/setup "{\"name\":\"owner\",\"password\":\"$PW\",\"setupCode\":\"$(tr -d '\r\n ' <"$SMOKE_OUT/data/setup-code.txt")\",\"acknowledgedLegal\":true}" | json "d['token']") \
  || fail "no administrator"
sapi POST /api/setup/complete '{}' >>"$LOG"

step "Peer to peer on, through our own WittyRelay"
sapi PUT /api/p2p/settings "{\"accept\":true,\"enabled\":true,\"port\":$P2P,\"devicesOnly\":true,\"layers\":{\"dht\":false,\"relay\":true,\"holePunch\":true,\"portMap\":false,\"ipv6\":false},\"ownRelays\":[{\"addr\":\"$RELAY\",\"key\":\"$KEY\"}]}" >>"$LOG"
echo >>"$LOG"
grep -q '"error"' "$LOG" && fail "the settings were refused: $(cat "$LOG")"
relayed() { sapi GET /api/p2p >"$SMOKE_OUT/p2p-info.json"; grep -q 'p2p-circuit' "$SMOKE_OUT/p2p-info.json"; }
until_ok 90 "the server holding a place on the relay (a p2p-circuit address)" relayed || { cat "$SMOKE_OUT/p2p-info.json" >>"$LOG"; exit 1; }
cat "$SMOKE_OUT/p2p-info.json" >>"$LOG"

step "Connect a device: an invite link"
INV=$(sapi POST /api/p2p/invite "{\"name\":\"Smoke phone\",\"password\":\"$PW\"}")
echo "$INV" | sed 's/s=[^&"]*/s=…/' >>"$LOG"
LINK=$(json "d.get('link') or d.get('url') or ''" <<<"$INV")
[ -n "$LINK" ] || fail "no link in the invite"
read -r SID SECRET HINTS < <(python3 - "$LINK" <<'EOF'
import sys, json, urllib.parse as u
p = u.urlparse(sys.argv[1]); q = u.parse_qs(p.query)
print(p.path.strip('/').split('/')[-1], q['s'][0], json.dumps(q.get('a', [])).replace(' ', ''))
EOF
)
echo "server $SID, $(python3 -c "import json,sys; print(len(json.loads(sys.argv[1])))" "$HINTS") addresses in the link" >>"$LOG"
grep -q p2p-circuit <<<"$HINTS" || fail "the link has no relay address: $HINTS"

step "At home: the link puts this device on the server's list"
DEVKEY=$("$build/bin/wittyconnect" keygen | json "d['key']")
mkfifo "$SMOKE_OUT/connect.in"
exec 7<>"$SMOKE_OUT/connect.in"
asuser sm-cli "$build/bin/wittyconnect" <"$SMOKE_OUT/connect.in" >"$SMOKE_OUT/connect.out" 2>"$SMOKE_OUT/connect.err" &
echo "{\"key\":\"$DEVKEY\",\"serverId\":\"$SID\",\"hints\":$HINTS,\"noDht\":true}" >&7
until_ok 60 "the connector ready" grep -q '"event":"ready"' "$SMOKE_OUT/connect.out" || exit 1
read -r PORT LSECRET < <(grep -m1 '"event":"ready"' "$SMOKE_OUT/connect.out" | json "d['port'], d['secret']" | tr -d "(),'")
echo "{\"cmd\":\"enrol\",\"secret\":\"$SECRET\",\"name\":\"Smoke phone\",\"platform\":\"android\"}" >&7
until_ok 90 "the device put on the server's list" grep -q '"event":"enrol"' "$SMOKE_OUT/connect.out" || exit 1
grep '"event":"enrol"' "$SMOKE_OUT/connect.out" | tee -a "$LOG" | grep -q '"ok":true' || fail "the server refused the invite: $(grep '"event":"enrol"' "$SMOKE_OUT/connect.out")"
L=http://127.0.0.1:$PORT
lstatus() { ns sm-cli curl -sS -m "$1" -H "X-Witty-Connect: $LSECRET" "$L/api/status"; }
lstatus 30 | tee -a "$LOG" | grep -q '"version"' || fail "no status through the connector"

step "Away: the cable to the server's home is unplugged, and the relay carries on"
ns sm-cli ip link set eth1 down
for a in 192.168.1.10 192.168.3.1; do
  if ns sm-cli curl -s -m 3 "http://$a:$HTTP/" >/dev/null 2>&1; then fail "away, the app still reaches $a directly"; fi
done
echo '{"cmd":"network-changed"}' >&7
answers() { lstatus 15 | grep -q '"version"'; }
relay_path() { grep '"event":"state"' "$SMOKE_OUT/connect.out" | tail -1 | grep -q '"path":"relay'; }
until_ok 120 "the server answering through the relay" answers || exit 1
until_ok 30 "the connector saying it goes through the relay" relay_path || { grep '"event":"state"' "$SMOKE_OUT/connect.out" | tail -3 >>"$LOG"; exit 1; }
grep '"event":"state"' "$SMOKE_OUT/connect.out" | tail -1 >>"$LOG"

step "Away: sign in and use the server through it"
tok=$(ns sm-cli curl -sS -m 30 -H "X-Witty-Connect: $LSECRET" -H 'Content-Type: application/json' -d "{\"name\":\"owner\",\"password\":\"$PW\"}" "$L/api/login" | json "d['token']") \
  || fail "sign-in through the connector"
code=$(ns sm-cli curl -s -o /dev/null -w '%{http_code}' -m 30 -H "X-Witty-Connect: $LSECRET" -H "Authorization: Bearer $tok" "$L/api/libraries")
[ "$code" = 200 ] || fail "libraries through the connector: $code"
# A web page from away (what a web view loads: WittyPlay's games frame is one) with the cookie alone.
code=$(ns sm-cli curl -s -o /dev/null -w '%{http_code}' -m 30 -H 'Accept: text/html' --cookie "wittyconnect=$LSECRET" "$L/")
[ "$code" = 200 ] || fail "the web app's page through the connector with its cookie: $code"

step "A device taken off the list is shut out"
dev=$(sapi GET /api/p2p/devices | json "[x['id'] for x in (d if isinstance(d, list) else d.get('devices', [])) if 'Smoke' in (x.get('name') or '')][0]") || fail "the device isn't on the list"
sapi DELETE "/api/p2p/devices/$dev" >>"$LOG"
gone() { [ "$(ns sm-cli curl -s -o /dev/null -w '%{http_code}' -m 10 -H "X-Witty-Connect: $LSECRET" "$L/api/status")" != 200 ]; }
until_ok 60 "the removed device refused" gone || exit 1
exec 7>&-
finish
