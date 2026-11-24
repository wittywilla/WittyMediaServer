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
#
# Then a third home, the owner's homelab (0.8.4's phone report): the server in a container on Docker's bridge
# network with only its web port published, no WittyRelay, and the phone on the same network; on the internet, an
# ordinary public relay, which carries an invite but not a film.
set -uo pipefail
build=$(cd "${1:?build folder}" && pwd)
here=$(cd "$(dirname "$0")/../.." && pwd)
. "$(dirname "$0")/lib.sh"
[ "$(id -u)" = 0 ] || { echo "needs root (network namespaces)"; exit 2; }
RUNAS=${SUDO_USER:-root}
HTTP=19790
P2P=19791
KEY=smoke-relay-key-$(head -c 9 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')
NS="sm-inet sm-relay sm-obs sm-rs sm-srv sm-rc sm-cli sm-pr sm-hr sm-hl sm-dh sm-dc sm-ph"
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

# A device that has never been at home: a new invite link alone gets it in, through the server's own relay (#614).
step "Away, never at home: a new link alone puts a device on the list, through the relay"
INV2=$(sapi POST /api/p2p/invite "{\"name\":\"Away phone\",\"password\":\"$PW\"}")
read -r SID2 SECRET2 HINTS2 < <(python3 - "$(json "d.get('link') or d.get('url') or ''" <<<"$INV2")" <<'EOF'
import sys, json, urllib.parse as u
p = u.urlparse(sys.argv[1]); q = u.parse_qs(p.query)
print(p.path.strip('/').split('/')[-1], q['s'][0], json.dumps(q.get('a', [])).replace(' ', ''))
EOF
)
[ -n "$SECRET2" ] || fail "no second invite"
# connector <name> <secret>: a new device's connector, started away, that shows the server that secret.
connector() {
  local n=$1 k
  k=$("$build/bin/wittyconnect" keygen | json "d['key']")
  rm -f "$SMOKE_OUT/c-$n.in"; mkfifo "$SMOKE_OUT/c-$n.in"
  asuser sm-cli "$build/bin/wittyconnect" <"$SMOKE_OUT/c-$n.in" >"$SMOKE_OUT/c-$n.out" 2>"$SMOKE_OUT/c-$n.err" &
  exec 8<>"$SMOKE_OUT/c-$n.in"
  echo "{\"key\":\"$k\",\"serverId\":\"$SID2\",\"hints\":$HINTS2,\"noDht\":true}" >&8
  until_ok 60 "connector $n ready" grep -q '"event":"ready"' "$SMOKE_OUT/c-$n.out" || return 1
  echo "{\"cmd\":\"enrol\",\"secret\":\"$2\",\"name\":\"$n\",\"platform\":\"android\"}" >&8
  until_ok 150 "connector $n answered" grep -q '"event":"enrol"' "$SMOKE_OUT/c-$n.out" || return 1
  grep '"event":"enrol"' "$SMOKE_OUT/c-$n.out" | sed 's/"code":"[0-9]*"/"code":"…"/' >>"$LOG"
}
connector away "$SECRET2" || exit 1
grep '"event":"enrol"' "$SMOKE_OUT/c-away.out" | grep -q '"ok":true' || fail "the link didn't get a device that was never at home listed"
read -r PORT2 LSECRET2 < <(grep -m1 '"event":"ready"' "$SMOKE_OUT/c-away.out" | json "d['port'], d['secret']" | tr -d "(),'")
away2() { ns sm-cli curl -sS -m 15 -H "X-Witty-Connect: $LSECRET2" "http://127.0.0.1:$PORT2/api/status" | grep -q '"version"'; }
until_ok 120 "the server answering it through the relay" away2 || exit 1
grep '"event":"state"' "$SMOKE_OUT/c-away.out" | tail -1 | tee -a "$LOG" | grep -q '"path":"relay' || fail "not through the relay"
exec 8>&-

step "A used link and a made-up one list nobody"
connector stranger-used "$SECRET2" || exit 1
exec 8>&-
connector stranger-madeup "madeUpSecret$(head -c 8 /dev/urandom | base64 | tr -dc A-Za-z0-9)xx" || exit 1
exec 8>&-
grep -h '"event":"enrol"' "$SMOKE_OUT"/c-stranger-*.out | grep -q '"ok":true' && fail "a used or made-up link listed a device"
sapi GET /api/p2p/devices | grep -q stranger && fail "a stranger is on the list"

# ---------------------------------------------------------------- the owner's homelab
WEB2=9797; P2P2=9798; HOST=192.168.5.50
step "The homelab: a server in a container, only its web port published, a phone on the same network"
{
  for n in sm-pr sm-hr sm-hl sm-dh sm-dc sm-ph; do ip netns add "$n"; ip -n "$n" link set lo up; done
  wire sm-pr eth0 80.0.0.30
  wire sm-hr wan 80.0.0.4
  ns sm-hl ip link add br0 type bridge; ns sm-hl ip link set br0 up
  lan() { # lan NS DEV ADDR: a cable to the homelab's own network
    i=$((i + 1)); ip link add "$2" netns "$1" type veth peer name "l$i" netns sm-hl
    ns sm-hl ip link set "l$i" master br0 up; ns "$1" ip addr add "$3/24" dev "$2"; ns "$1" ip link set "$2" up
  }
  lan sm-hr lan 192.168.5.1
  lan sm-dh eth0 $HOST
  lan sm-ph eth0 192.168.5.20
  ns sm-dh ip route add default via 192.168.5.1; ns sm-ph ip route add default via 192.168.5.1
  # The home router: one outside address, nothing forwarded in.
  ns sm-hr sysctl -qw net.ipv4.ip_forward=1
  ns sm-hr iptables -t nat -A POSTROUTING -o wan -j MASQUERADE
  ns sm-hr iptables -P FORWARD DROP
  ns sm-hr iptables -A FORWARD -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  ns sm-hr iptables -A FORWARD -i lan -o wan -j ACCEPT
  # Docker's bridge on the homelab: the container behind the host's NAT, as `ports: - "9797:9797"` publishes it.
  ip link add docker0 netns sm-dh type veth peer name eth0 netns sm-dc
  ns sm-dh ip addr add 172.17.0.1/16 dev docker0; ns sm-dh ip link set docker0 up
  ns sm-dc ip addr add 172.17.0.2/16 dev eth0; ns sm-dc ip link set eth0 up; ns sm-dc ip route add default via 172.17.0.1
  ns sm-dh sysctl -qw net.ipv4.ip_forward=1
  ns sm-dh iptables -t nat -A POSTROUTING -s 172.17.0.0/16 ! -o docker0 -j MASQUERADE
  ns sm-dh iptables -t nat -A PREROUTING -p tcp --dport $WEB2 -m addrtype --dst-type LOCAL -j DNAT --to-destination 172.17.0.2:$WEB2
} >>"$LOG" 2>&1 || fail "the homelab's networks could not be made"
ns sm-pr "$build/bin/p2p-netsim" public-relay -ip 80.0.0.30 -out "$SMOKE_OUT/public-relay.txt" >"$SMOKE_OUT/public-relay.log" 2>&1 &
until_ok 15 "the public relay's address" grep -q quic-v1 "$SMOKE_OUT/public-relay.txt" || exit 1
PRELAY=$(grep -m1 quic-v1 "$SMOKE_OUT/public-relay.txt")
mkdir -p "$SMOKE_OUT/homelab"; chown "$RUNAS" "$SMOKE_OUT/homelab"
# As the image runs it: WMS_DOCKER says it's a container, WMS_P2P_PORT is the port compose files publish.
asuser sm-dc env WMS_NO_BROWSER=1 WMS_DOCKER=1 WMS_P2P_PORT=$P2P2 "$build/bin/wittymediaserver" serve -data "$SMOKE_OUT/homelab" -listen "0.0.0.0:$WEB2" \
  -open-browser=false </dev/null >"$SMOKE_OUT/homelab.log" 2>&1 &
S2=http://127.0.0.1:$WEB2
hapi() { ns sm-dc curl -sS -m 30 -X "$1" -H "Authorization: Bearer ${TOK2:-}" -H 'Content-Type: application/json' ${3:+--data "$3"} "$S2$2"; }
# The owner's browser, on the homelab's network: the page is opened at the host's address.
browser() { ns sm-ph curl -sS -m 30 -X "$1" -H "Authorization: Bearer ${TOK2:-}" -H 'Content-Type: application/json' ${3:+--data "$3"} "http://$HOST:$WEB2$2"; }
until_ok 60 "the homelab's server answers" ns sm-dc curl -fsS -m 2 "$S2/api/health" || exit 1
until_ok 30 "its setup code" test -s "$SMOKE_OUT/homelab/setup-code.txt" || exit 1
TOK2=$(hapi POST /api/setup "{\"name\":\"owner\",\"password\":\"$PW\",\"setupCode\":\"$(tr -d '\r\n ' <"$SMOKE_OUT/homelab/setup-code.txt")\",\"acknowledgedLegal\":true}" | json "d['token']") \
  || fail "no administrator on the homelab"
hapi POST /api/setup/complete '{}' >>"$LOG"
ns sm-ph curl -fsS -m 5 -o /dev/null "http://$HOST:$WEB2/api/health" || fail "the phone can't reach the published web port"
# Peer to peer with its defaults. Nothing here has the public address book, which is what introduces a server to
# the public network's nodes; naming the public relay as an own relay stands in for it (the server dials it, its
# registration fails: it isn't a WittyRelay), and from then on, as on the internet, that node tells the server it
# can't be reached and the server holds a place on it like any public node's. The phone is never told about it.
hapi PUT /api/p2p/settings "{\"accept\":true,\"enabled\":true,\"staticRelays\":[\"$PRELAY\"],\"ownRelays\":[{\"addr\":\"$PRELAY\",\"key\":\"stand-in-for-the-address-book\"}]}" >>"$LOG"
hrelayed() { hapi GET /api/p2p >"$SMOKE_OUT/homelab-p2p.json"; grep -q 'p2p-circuit' "$SMOKE_OUT/homelab-p2p.json"; }
until_ok 120 "a place on the public relay" hrelayed || { cat "$SMOKE_OUT/homelab-p2p.json" >>"$LOG"; exit 1; }
grep -q "/tcp/$P2P2\"" "$SMOKE_OUT/homelab-p2p.json" || fail "the peer-to-peer port isn't WMS_P2P_PORT's $P2P2"
browser GET /api/connect | tee -a "$LOG" | grep -q "$P2P2 (UDP and TCP)" || fail "the page doesn't say which port the container must publish"

# offer: Connect a device in the owner's browser. Sets CODE, LINK, QR.
offer() {
  local o; o=$(browser POST /api/connect/offer "{\"password\":\"$PW\"}")
  echo "$o" | sed 's/s=[^&"]*/s=…/g; s/c=[^&"]*/c=…/g; s/"code":"[^"]*"/"code":"…"/' >>"$LOG"
  read -r CODE LINK QR HOMES < <(json "d.get('code',''), d.get('link',''), d.get('qrLink',''), ' '.join(d.get('homeAddresses',[]))" <<<"$o" | tr -d "(),'")
}
parse() { python3 - "$1" <<'EOF'
import sys, json, urllib.parse as u
p = u.urlparse(sys.argv[1]); q = u.parse_qs(p.query)
print(p.path.strip('/').split('/')[-1], q['s'][0], json.dumps(q.get('a', [])).replace(' ', ''), (q.get('h') or ['-'])[0], (q.get('c') or ['-'])[0])
EOF
}

step "At home: the QR code holds the host's address and the code, and they sign the phone in"
offer
[ "$HOMES" = "http://$HOST:$WEB2" ] || fail "the address at home is '$HOMES', not the host's (a container's own address leads nowhere)"
read -r _ _ _ QRHOME QRCODE < <(parse "$QR")
[ "$QRHOME" = "http://$HOST:$WEB2" ] && [ "$QRCODE" != - ] || fail "the QR code holds no address at home or no code: $(sed 's/s=[^&]*/s=…/; s/c=[^&]*/c=…/' <<<"$QR")"
[ "${#QR}" -le 210 ] || fail "the QR link is ${#QR} bytes, more than the page's QR codes hold"
ns sm-ph python3 "$(dirname "$0")/webjoin.py" "$QRHOME" "$QRCODE" "Homelab phone" android | tee -a "$LOG" | grep -q '"ok": true' || fail "the QR code's address and code didn't sign the phone in"

step "Only a public relay between them: the invite gets through, and the app is told what didn't"
offer
read -r HSID HSECRET HHINTS _ _ < <(parse "$LINK")
grep -q "/ip4/$HOST/udp/$P2P2/quic-v1" <<<"$HHINTS" || fail "the link doesn't try the peer-to-peer port at the host: $HHINTS"
# The app's order (Android 0.6.x): a path first, then the invite, then the server proving its key.
k=$("$build/bin/wittyconnect" keygen | json "d['key']")
mkfifo "$SMOKE_OUT/c-homelab.in"; exec 9<>"$SMOKE_OUT/c-homelab.in"
asuser sm-ph "$build/bin/wittyconnect" <"$SMOKE_OUT/c-homelab.in" >"$SMOKE_OUT/c-homelab.out" 2>"$SMOKE_OUT/c-homelab.err" &
echo "{\"key\":\"$k\",\"serverId\":\"$HSID\",\"hints\":$HHINTS,\"noDht\":true}" >&9
until_ok 60 "the phone's connector ready" grep -q '"event":"ready"' "$SMOKE_OUT/c-homelab.out" || exit 1
read -r HPORT HLSECRET < <(grep -m1 '"event":"ready"' "$SMOKE_OUT/c-homelab.out" | json "d['port'], d['secret']" | tr -d "(),'")
hpath() { grep '"event":"state"' "$SMOKE_OUT/c-homelab.out" | tail -1 | grep -q '"path":"relayed"'; }
until_ok 60 "the phone finding the server through the public relay" hpath || exit 1
echo "{\"cmd\":\"enrol\",\"secret\":\"$HSECRET\",\"name\":\"Homelab phone 2\",\"platform\":\"android\"}" >&9
until_ok 90 "the invite answered" grep -q '"event":"enrol"' "$SMOKE_OUT/c-homelab.out" || exit 1
grep '"event":"enrol"' "$SMOKE_OUT/c-homelab.out" | sed 's/"code":"[0-9]*"/"code":"…"/' | tee -a "$LOG" | grep -q '"ok":true' || fail "the invite didn't get through the public relay"
# What the app shows in place of "Couldn't reach your server": the connector's own words for the step that failed.
ns sm-ph curl -sS -m 40 -D "$SMOKE_OUT/homelab-identity.h" -o "$SMOKE_OUT/homelab-identity.json" -H "X-Witty-Connect: $HLSECRET" \
  -H 'Content-Type: application/json' --data "{\"nonce\":\"$(head -c 32 /dev/urandom | base64 -w0)\"}" "http://127.0.0.1:$HPORT/api/identity" >>"$LOG" 2>&1
cat "$SMOKE_OUT/homelab-identity.json" >>"$LOG"
grep -qi '^x-witty-connect-error' "$SMOKE_OUT/homelab-identity.h" && grep -q 'direct connection' "$SMOKE_OUT/homelab-identity.json" \
  || fail "the connector didn't say why the server can't be used through a public relay: $(head -c 300 "$SMOKE_OUT/homelab-identity.json")"
exec 9>&-

step "Its peer-to-peer port published as well (the compose files since 0.8.5): the phone connects peer to peer at home"
ns sm-dh iptables -t nat -A PREROUTING -p tcp --dport $P2P2 -m addrtype --dst-type LOCAL -j DNAT --to-destination 172.17.0.2:$P2P2 >>"$LOG" 2>&1
ns sm-dh iptables -t nat -A PREROUTING -p udp --dport $P2P2 -m addrtype --dst-type LOCAL -j DNAT --to-destination 172.17.0.2:$P2P2 >>"$LOG" 2>&1
offer
read -r HSID HSECRET HHINTS _ _ < <(parse "$LINK")
k=$("$build/bin/wittyconnect" keygen | json "d['key']")
mkfifo "$SMOKE_OUT/c-homelab2.in"; exec 9<>"$SMOKE_OUT/c-homelab2.in"
asuser sm-ph "$build/bin/wittyconnect" <"$SMOKE_OUT/c-homelab2.in" >"$SMOKE_OUT/c-homelab2.out" 2>"$SMOKE_OUT/c-homelab2.err" &
echo "{\"key\":\"$k\",\"serverId\":\"$HSID\",\"hints\":$HHINTS,\"noDht\":true}" >&9
until_ok 60 "the phone's connector ready" grep -q '"event":"ready"' "$SMOKE_OUT/c-homelab2.out" || exit 1
read -r HPORT HLSECRET < <(grep -m1 '"event":"ready"' "$SMOKE_OUT/c-homelab2.out" | json "d['port'], d['secret']" | tr -d "(),'")
echo "{\"cmd\":\"enrol\",\"secret\":\"$HSECRET\",\"name\":\"Homelab phone 3\",\"platform\":\"android\"}" >&9
until_ok 90 "the invite answered" grep -q '"event":"enrol"' "$SMOKE_OUT/c-homelab2.out" || exit 1
grep '"event":"enrol"' "$SMOKE_OUT/c-homelab2.out" | sed 's/"code":"[0-9]*"/"code":"…"/' | tee -a "$LOG" | grep -q '"ok":true' || fail "the invite was refused"
hstatus() { ns sm-ph curl -sS -m 15 -H "X-Witty-Connect: $HLSECRET" "http://127.0.0.1:$HPORT/api/status" | grep -q '"version"'; }
until_ok 60 "the server answering the phone peer to peer" hstatus || exit 1
grep '"event":"state"' "$SMOKE_OUT/c-homelab2.out" | tail -1 | tee -a "$LOG" | grep -q "\"path\":\"local\".*$HOST" || fail "not straight to the host's published port"
exec 9>&-
finish
