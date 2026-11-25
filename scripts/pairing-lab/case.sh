#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# One pairing-lab case: a real server, a real WittyRelay and the phone's connector library, in the
# networks net.sh makes, paired once, by a link (what the QR code holds) or by a code, on the first
# try. Every step on both sides goes to <out>/<case>/steps.jsonl with its time.
#
#   sudo -E scripts/pairing-lab/case.sh <build folder> <results folder> <case>
#
# <case> is network.server.only.phone.way, for example symmetric-relay.restart.on.fresh.link:
#   network  see net.sh
#   server   fresh    the server has just started: the invite is made as soon as it answers
#            settled  the server has been up a while (its relay place is held) when the invite is made
#            restart  the server restarts between the invite (or the code shown) and the claim
#   only     on|off   "Only paired devices" (devicesOnly)
#   phone    fresh | upgrade | previous   (labphone/main.go says what each carries)
#   way      link | code
# The build folder holds bin/wittymediaserver, bin/wittyrelay and bin/labphone. Exit 0: paired.
set -uo pipefail
BUILD=$(cd "${1:?build folder}" && pwd)
OUTROOT=${2:?results folder}
CASE=${3:?case}
HERE=$(cd "$(dirname "$0")" && pwd)
[ "$(id -u)" = 0 ] || { echo "needs root (network namespaces)"; exit 2; }
RUNAS=${SUDO_USER:-${PL_USER:-root}}
PL_HTTP=19890
PL_P2P=19891
PL_HTTP_B=19892
PL_P2P_B=19893
export PL_P2P
# shellcheck source=net.sh
. "$HERE/net.sh"

IFS=. read -r NET SSTATE ONLY CSTATE WAY <<<"$CASE"
[ -n "$WAY" ] || { echo "case must be network.server.only.phone.way"; exit 2; }
mkdir -p "$OUTROOT"
OUT=$(cd "$OUTROOT" && pwd)/$CASE
rm -rf "$OUT"; mkdir -p "$OUT"
STEPS=$OUT/steps.jsonl
# The phone runs as an ordinary user and writes its steps here too.
: >"$STEPS"; : >"$OUT/prep-steps.jsonl"; chmod 666 "$STEPS" "$OUT/prep-steps.jsonl"
T0=$(date +%s%3N)
KEY=lab-relay-key-$(head -c 9 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')
UH=$(getent passwd "$RUNAS" | cut -d: -f6)
asuser() { local n=$1; shift; ip netns exec "$n" sudo -u "$RUNAS" env HOME="$UH" "$@"; }

# sstep <name> <ok:true|false> [detail json]: a server-side step, on the same clock as the phone's.
sstep() {
  printf '{"t":%s,"at":"%s","side":"%s","step":"%s","ok":%s,"detail":%s}\n' \
    "$(($(date +%s%3N) - T0))" "$(date -u +%FT%T.%3NZ)" "${SIDE:-server}" "$1" "$2" "${3:-null}" >>"$STEPS"
}
die() {
  sstep "lab-error" false "$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$*")"
  echo "$CASE: lab error: $*" >&2
  finish 3
}
jget() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)" 2>/dev/null; }
jstr() { python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$1"; }

POLLER=
finish() {
  local rc=$1
  [ -n "$POLLER" ] && kill "$POLLER" 2>/dev/null
  for f in "$OUT"/server*.pid; do [ -f "$f" ] && kill "$(cat "$f")" 2>/dev/null; done
  sleep 0.5
  for f in "$OUT"/server*.pid; do [ -f "$f" ] && kill -9 "$(cat "$f")" 2>/dev/null; done
  lab_net_down
  rm -rf "/tmp/plab-xdg.$$"
  python3 "$HERE/report.py" case "$OUT" "$rc" >/dev/null 2>&1
  chmod -R a+rX "$OUT" 2>/dev/null
  exit "$rc"
}
trap 'sstep case-stopped false "\"stopped from outside: the time limit (PL_CASE_TIMEOUT) or Ctrl-C\""; finish 4' INT TERM

# ---------------------------------------------------------------- the networks and the relay
UPLINK=
[ "$WAY" = code ] && UPLINK=uplink
if ! lab_net "$NET" "$UPLINK" >"$OUT/net.log" 2>&1; then
  die "the networks could not be made: $(tail -3 "$OUT/net.log")"
fi
sstep network-ready true "{\"network\":\"$NET\",\"uplink\":\"${UPLINK:-no}\",\"ipv6\":\"$PL_V6\"}"

RELAY=
PUBLIC_RELAY=
if [ "$PL_RELAY_KIND" = public ]; then
  # An ordinary public relay: the server finds it as it would one on the public network (here named in
  # Settings, Remote Access as a relay to use, since this network has no address book to find it in).
  asuser pl-relay "$BUILD/bin/labphone" -public-relay "/ip4/$PL_RELAY4/udp/4001/quic-v1,/ip4/$PL_RELAY4/tcp/4001" >"$OUT/relay.log" 2>&1 &
  for _ in $(seq 1 75); do grep -q 'relay address' "$OUT/relay.log" 2>/dev/null && break; sleep 0.2; done
  PUBLIC_RELAY=$(grep -m1 -o '/ip4/[^ ]*/udp/4001/quic-v1/p2p/12D3KooW[1-9A-HJ-NP-Za-km-z]*' "$OUT/relay.log")
  [ -n "$PUBLIC_RELAY" ] || die "the public relay didn't start: $(tail -3 "$OUT/relay.log")"
  sstep relay-ready true "{\"publicRelay\":\"$PUBLIC_RELAY\"}"
elif [ -n "$PL_RELAY_ADDRS" ]; then
  mkdir -p "$OUT/relay"; chown "$RUNAS" "$OUT/relay"
  RELAY_HTTP=$PL_RELAY4:8080; [ "$PL_V6" = yes ] && RELAY_HTTP="[$PL_RELAY6]:8080"
  asuser pl-relay env WITTYRELAY_DATA_DIR="$OUT/relay" WITTYRELAY_HTTP="$RELAY_HTTP" WITTYRELAY_KEYS="$KEY" \
    WITTYRELAY_ANNOUNCE="$PL_RELAY_ADDRS" WITTYRELAY_LOG_LEVEL=debug "$BUILD/bin/wittyrelay" >"$OUT/relay.log" 2>&1 &
  for _ in $(seq 1 75); do grep -q '"relay address' "$OUT/relay.log" 2>/dev/null && break; sleep 0.2; done
  RID=$(grep -o '12D3KooW[1-9A-HJ-NP-Za-km-z]*' "$OUT/relay.log" | head -1)
  [ -n "$RID" ] || die "the relay didn't start: $(tail -3 "$OUT/relay.log")"
  RELAY=${PL_RELAY_ADDRS%%,*}/p2p/$RID
  sstep relay-ready true "{\"relay\":\"$RELAY\"}"
fi

# ---------------------------------------------------------------- a server
# start_server <tag> <http port> <p2p port>: started fresh or again on the same data, its PID kept.
start_server() {
  local tag=$1 http=$2 ff
  mkdir -p "$OUT/data-$tag"; chown "$RUNAS" "$OUT" "$OUT/data-$tag"
  ff=$(command -v ffmpeg || true)
  ip netns exec pl-srv sudo -u "$RUNAS" env HOME="$UH" WMS_NO_BROWSER=1 ${PL_IN_DOCKER:+WMS_DOCKER=1} ${ff:+WMS_FFMPEG=$ff} GOLOG_LOG_LEVEL="${GOLOG_LOG_LEVEL:-error,p2p-holepunch=debug,autorelay=debug}" \
    sh -c 'echo $$ >"$0"; exec "$@"' "$OUT/server-$tag.pid" "$BUILD/bin/wittymediaserver" serve -data "$OUT/data-$tag" \
    -listen "0.0.0.0:$http" -open-browser=false </dev/null >>"$OUT/server-$tag.log" 2>&1 &
  local i
  for i in $(seq 1 120); do
    ip netns exec pl-srv curl -fsS -m 2 "http://127.0.0.1:$http/api/health" >/dev/null 2>&1 && return 0
    sleep 0.5
  done
  return 1
}
stop_server() { # by its own PID, never by name
  local f=$OUT/server-$1.pid p
  p=$(cat "$f" 2>/dev/null) || return 0
  kill "$p" 2>/dev/null
  for _ in $(seq 1 40); do kill -0 "$p" 2>/dev/null || break; sleep 0.25; done
  kill -9 "$p" 2>/dev/null
  rm -f "$f"
}
# api <tag> <method> <path> [json]
api() {
  local port=$PL_HTTP tokvar=TOK_$1 where=pl-srv base
  [ "$1" = b ] && port=$PL_HTTP_B
  base=http://127.0.0.1:$port
  # A server in a container is used the way its owner uses it: from a computer at home, at the host's
  # published address (what Connect a device takes the home network's address from).
  if [ "$1" = a ] && [ -n "${PL_HOME_URL:-}" ]; then where=pl-rs; base=$PL_HOME_URL; fi
  ip netns exec "$where" curl -sS -m 60 -X "$2" -H "Authorization: Bearer ${!tokvar:-}" -H 'Content-Type: application/json' \
    ${4:+--data "$4"} "$base$3"
}
PW=$(head -c 18 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')x9
# setup_server <tag> <p2p port>: an administrator, then peer to peer on as the owner would set it.
setup_server() {
  local tag=$1 p2p=$2 code tok only=true dht=false relays='[]' relayon=true
  for _ in $(seq 1 60); do [ -s "$OUT/data-$tag/setup-code.txt" ] && break; sleep 0.5; done
  code=$(tr -d '\r\n ' <"$OUT/data-$tag/setup-code.txt" 2>/dev/null)
  tok=$(api "$tag" POST /api/setup "{\"name\":\"owner\",\"password\":\"$PW\",\"setupCode\":\"$code\",\"acknowledgedLegal\":true}" | jget "d['token']")
  [ -n "$tok" ] || return 1
  printf -v "TOK_$tag" '%s' "$tok"
  api "$tag" POST /api/setup/complete '{}' >/dev/null
  [ "$ONLY" = off ] && only=false
  [ "$WAY" = code ] && dht=true
  [ -n "$RELAY" ] && relays="[{\"addr\":\"$RELAY\",\"key\":\"$KEY\"}]"
  local static='[]'
  if [ -n "$PUBLIC_RELAY" ]; then
    # Nothing here has the public address book, which is what introduces a server to the public network's
    # nodes. Naming the public relay as an own relay too stands in for it (as scripts/smoke/remote.sh does):
    # the server dials it, its registration fails, and from then on that node tells the server it can't be
    # reached (AutoNAT) and the server holds a place on it like on any public node.
    static="[\"$PUBLIC_RELAY\"]"
    relays="[{\"addr\":\"$PUBLIC_RELAY\",\"key\":\"stand-in-for-the-address-book\"}]"
  fi
  local body="{\"accept\":true,\"enabled\":true,\"port\":$p2p,\"devicesOnly\":$only,\"layers\":{\"dht\":$dht,\"relay\":$relayon,\"holePunch\":true,\"portMap\":false,\"ipv6\":true},\"ownRelays\":$relays,\"staticRelays\":$static}"
  api "$tag" PUT /api/p2p/settings "$body" >"$OUT/p2p-settings-$tag.json"
  python3 -c "import json,sys; d=json.load(open(sys.argv[1])); sys.exit(0 if isinstance(d, dict) and 'settings' in d else 1)" "$OUT/p2p-settings-$tag.json"
}
# wait_p2p <tag> <relayed:yes|no> <seconds>: until peer to peer answers (and, for yes, a place on the relay).
wait_p2p() {
  local i
  for i in $(seq 1 $(($3 * 2))); do
    api "$1" GET /api/p2p >"$OUT/p2p-$1.json" 2>/dev/null
    if grep -q '"serverId"' "$OUT/p2p-$1.json"; then
      [ "$2" = no ] && return 0
      grep -q 'p2p-circuit' "$OUT/p2p-$1.json" && return 0
    fi
    sleep 0.5
  done
  return 1
}
# make_invite <tag> [plain]: prints the link the QR code holds (plain: the link to paste instead). As the owner does it: Settings, Remote Access, Connect a
# device (POST /api/connect/offer: a code and a link together), the QR code's link; the plain invite (POST
# /api/p2p/invite) on a server without Connect a device.
make_invite() {
  api "$1" POST /api/connect/offer "{\"password\":\"$PW\"}" >"$OUT/invite-$1.json"
  local l
  if [ "${2:-qr}" = plain ]; then l=$(jget "d.get('link') or ''" <"$OUT/invite-$1.json")
  else l=$(jget "d.get('qrLink') or d.get('link') or ''" <"$OUT/invite-$1.json"); fi
  if [ -z "$l" ]; then
    api "$1" POST /api/p2p/invite "{\"name\":\"Lab phone\",\"password\":\"$PW\"}" >"$OUT/invite-$1.json"
    l=$(jget "d.get('link') or ''" <"$OUT/invite-$1.json")
  fi
  echo "$l"
}
redact_link() { sed 's/s=[^&"]*/s=…/' <<<"$1"; }

# The server's side, every second while the case runs: what /api/p2p and the device list say, when it changes.
poll_server() {
  local last="" now
  while sleep 1; do
    now=$( { api a GET /api/p2p; echo; api a GET '/api/p2p/devices?all=1'; } 2>/dev/null | python3 -c '
import json, sys
parts = [l for l in sys.stdin.read().splitlines() if l.strip()]
out = {}
try:
    p = json.loads(parts[0]); a = p.get("addrs") or []
    out["addrs"] = len(a); out["relayed"] = any("p2p-circuit" in x for x in a)
    for k in ("enabled", "devicesOnly", "reachability", "relays", "connections", "peers"):
        if k in p: out[k] = p[k]
except Exception as e: out["p2p"] = "unreadable"
try:
    d = json.loads(parts[1]); d = d if isinstance(d, list) else d.get("devices", [])
    out["devices"] = [{"name": x.get("name"), "how": x.get("how"), "lastSeen": x.get("lastSeen"), "lastPath": x.get("lastPath")} for x in d]
except Exception: out["devices"] = "unreadable"
print(json.dumps(out, sort_keys=True))' 2>/dev/null)
    [ -n "$now" ] && [ "$now" != "$last" ] && { sstep server-state true "$now"; last=$now; }
  done
}

# ---------------------------------------------------------------- the phone's state before pairing
PHONE=$OUT/phone; mkdir -p "$PHONE"; chown "$RUNAS" "$PHONE"
phone() { asuser pl-cli env GOLOG_LOG_LEVEL="${PL_PHONE_GOLOG:-error}" "$BUILD/bin/labphone" -t0 "$T0" -order "${PL_ORDER:-0.6.3}" -steps "$STEPS" -statedir "$PHONE" "$@"; }

# PL_PHONE=emulator: the real app instead (PL_APK, and PL_OLD_APK for an upgrade), on an emulator started
# inside the phone's namespace (AVD PL_AVD, console port PL_EMU_PORT), so its traffic crosses the phone's router.
EMU=
if [ "${PL_PHONE:-lib}" = emulator ]; then
  [ "$CSTATE" = previous ] && die "the emulator covers fresh and upgrade; previous is the library phone's"
  [ "$WAY" = code ] || [ -n "${PL_APK:-}" ] || die "PL_APK is needed"
  EMU_PORT=${PL_EMU_PORT:-5770}
  EMU=emulator-$EMU_PORT
  SDK=${ANDROID_SDK_ROOT:-$UH/android-sdk}
  # Its own runtime folder: the emulator then starts its own netsimd (the Wi-Fi it carries) inside this
  # namespace, rather than joining one another emulator on this machine started outside it.
  XDG=/tmp/plab-xdg.$$; mkdir -p "$XDG"; chown "$RUNAS" "$XDG"; chmod 700 "$XDG"  # short: sockets live here
  AVDS=${ANDROID_AVD_HOME:-$UH/.android/avd}
  emu() { asuser pl-cli env ANDROID_SDK_ROOT="$SDK" ANDROID_HOME="$SDK" ANDROID_AVD_HOME="$AVDS" ANDROID_ADB_SERVER_PORT=5099 XDG_RUNTIME_DIR="$XDG" TMPDIR="$XDG" PATH="$SDK/platform-tools:$PATH" "$@"; }
  if [ "$CSTATE" = upgrade ]; then
    # Home first: a cable straight from the phone to the server's computer, unplugged before pairing.
    ip link add eth1 netns pl-cli type veth peer name eth1 netns pl-srv
    nsx pl-srv ip addr add 192.168.3.1/24 dev eth1; nsx pl-srv ip link set eth1 up
    nsx pl-cli ip addr add 192.168.3.2/24 dev eth1; nsx pl-cli ip link set eth1 up
  fi
  t=$(date +%s%3N)
  emu "$SDK/emulator/emulator" -avd "${PL_AVD:-plab-phone}" -port "$EMU_PORT" -no-window -no-audio -no-boot-anim \
    -no-snapshot -gpu "${PL_EMU_GPU:-swangle_indirect}" -cores 2 -memory 3072 ${PL_EMU_WIPE:+-wipe-data} >"$OUT/emulator.log" 2>&1 &
  booted=
  for _ in $(seq 1 $((${PL_EMU_BOOT:-240} / 2))); do
    grep -qE "Unknown AVD|FATAL" "$OUT/emulator.log" 2>/dev/null && break
    [ "$(emu adb -s "$EMU" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = 1 ] && { booted=1; break; }
    sleep 2
  done
  [ -n "$booted" ] || die "the emulator didn't boot: $(tail -5 "$OUT/emulator.log")"
  [ "$(emu adb -s "$EMU" emu avd name 2>/dev/null | head -1 | tr -d '\r')" = "${PL_AVD:-plab-phone}" ] || die "$EMU isn't this lab's emulator"
  SIDE=phone sstep emulator-booted true "{\"ms\":$(($(date +%s%3N) - t)),\"serial\":\"$EMU\"}"
fi

if [ "$CSTATE" = previous ]; then
  # Paired before with another server (B, in the same home), which is still running.
  SIDE=prep sstep prep-other-server true null
  start_server b "$PL_HTTP_B" || die "server B didn't start"
  setup_server b "$PL_P2P_B" || die "server B: settings refused: $(cat "$OUT/p2p-settings-b.json")"
  wait_p2p b "$([ -n "$RELAY$PUBLIC_RELAY" ] && echo yes || echo no)" 90 || die "server B: no peer to peer"
  LB=$(make_invite b plain); [ -n "$LB" ] || die "server B: no invite"
  T_PREP=$(date +%s%3N)
  # The order that gets through a relay: this is only the setting-up, not what is tested.
  if asuser pl-cli "$BUILD/bin/labphone" -order enrol-first -mode link -link "$LB" -state fresh -statedir "$PHONE" -steps "$OUT/prep-steps.jsonl" -name "Lab phone" \
    >"$OUT/prep-phone.log" 2>&1; then
    SIDE=prep sstep prep-paired-with-other true "{\"ms\":$(($(date +%s%3N) - T_PREP))}"
  else
    die "the phone couldn't pair with server B first (prep): $(tail -2 "$OUT/prep-phone.log")"
  fi
fi

# ---------------------------------------------------------------- the server, and the invite or code
start_server a "$PL_HTTP" || die "the server didn't start: $(tail -5 "$OUT/server-a.log")"
sstep server-started true "{\"pid\":$(cat "$OUT/server-a.pid")}"
setup_server a "$PL_P2P" || die "settings refused: $(cat "$OUT/p2p-settings-a.json")"
sstep p2p-on true "{\"devicesOnly\":\"$ONLY\",\"ownRelay\":$([ -n "$RELAY" ] && echo true || echo false),\"dht\":$([ "$WAY" = code ] && echo true || echo false)}"
POLLER_START() { poll_server & POLLER=$!; }

EMUPID=
if [ -n "$EMU" ]; then
  # The app is opened now (and, for an upgrade, used at home first); the link reaches it later, by file.
  chown "$RUNAS" "$OUT"
  emu python3 "$HERE/emuphone.py" --serial "$EMU" --apk "${PL_APK:-}" --old-apk "${PL_OLD_APK:-}" --mode "$WAY" \
    --linkfile "$PHONE/link.txt" --codefile "$PHONE/code.txt" --state "$CSTATE" --steps "$STEPS" --t0 "$T0" --out "$OUT" \
    --home "http://192.168.3.1:$PL_HTTP" --user owner --password "$PW" >"$OUT/phone.log" 2>&1 &
  EMUPID=$!
  if [ "$CSTATE" = upgrade ]; then
    for _ in $(seq 1 300); do [ -f "$OUT/at-home.done" ] || ! kill -0 "$EMUPID" 2>/dev/null && break; sleep 1; done
    [ -f "$OUT/at-home.done" ] || { wait "$EMUPID"; die "the older app couldn't sign in at home: $(tail -3 "$OUT/phone.log")"; }
    nsx pl-cli ip link del eth1
    SIDE=prep sstep unplugged-from-home true "$(api a GET '/api/p2p/devices?all=1' | head -c 600 | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')"
    touch "$OUT/away.done"
  fi
fi

if [ "$SSTATE" = fresh ]; then
  wait_p2p a no 60 || die "peer to peer never came up"
else
  if wait_p2p a "$([ -n "$RELAY$PUBLIC_RELAY" ] && echo yes || echo no)" 90; then
    sstep settled true "$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(json.dumps({"addrs": d.get("addrs")}))' "$OUT/p2p-a.json")"
  else
    sstep settled false "$(jstr "no place on the relay after 90 s: $(head -c 400 "$OUT/p2p-a.json")")"
  fi
  [ -z "$RELAY$PUBLIC_RELAY" ] && sleep 5
fi
SID=$(jget "d['serverId']" <"$OUT/p2p-a.json")
# What Connect a device says about connections from away (and, in a container, which port to publish).
sstep connect-status true "$(api a GET /api/connect | python3 -c 'import json,sys
try: d = json.load(sys.stdin)
except Exception: d = {}
print(json.dumps({k: d.get(k) for k in ("away", "words", "hint", "detail") if d.get(k)}))' 2>/dev/null || echo null)"
POLLER_START

if [ "$CSTATE" = upgrade ] && [ -z "$EMU" ]; then
  # An older app's leftovers: its key, the local port it used, and addresses this server had
  # before it restarted on other ports.
  # The key is made the way the older app made it (the same library call).
  K=$(asuser pl-cli "$BUILD/bin/labphone" -newkey 2>/dev/null)
  [ -n "$K" ] || die "no key for the upgrade leftovers"
  python3 - "$PHONE/phone.json" "$SID" "$PL_SRV_IP" "$K" <<'EOF'
import json, sys
path, sid, ip, key = sys.argv[1:5]
fam = "ip6" if ":" in ip else "ip4"
json.dump({"key": key, "port": 38411, "servers": {sid: [f"/{fam}/{ip}/udp/40411/quic-v1/p2p/{sid}", f"/{fam}/{ip}/tcp/40411/p2p/{sid}"]}, "previous": ""}, open(path, "w"))
EOF
  chown "$RUNAS" "$PHONE/phone.json"
  SIDE=prep sstep prep-upgrade-leftovers true "{\"port\":38411,\"staleAddrs\":2}"
fi

restart_server() {
  local t=$(($(date +%s%3N)))
  stop_server a
  sstep server-stopped true null
  start_server a "$PL_HTTP" || die "the server didn't start again"
  # Signed in before the restart: the same sign-in, if it survived; else signing in again.
  if ! api a GET /api/p2p | grep -q serverId; then
    TOK_a=$(api a POST /api/login "{\"name\":\"owner\",\"password\":\"$PW\"}" | jget "d['token']")
  fi
  wait_p2p a no 60
  sstep server-restarted true "{\"ms\":$(($(date +%s%3N) - t)),\"pid\":$(cat "$OUT/server-a.pid")}"
}

RC=0
link_step() { # link_step <step> <link>: what a link offers the phone
  sstep "$1" true "$(python3 -c '
import json, sys, urllib.parse as u
l = sys.argv[1]; q = u.parse_qs(u.urlparse(l).query); a = q.get("a", [])
print(json.dumps({"addrs": a, "relayed": any("p2p-circuit" in x for x in a), "home": q.get("h", [""])[0], "bytes": len(l), "link": l.split("?")[0]}))' "$2")"
}
case "$WAY" in
  # link: Connect a device's QR code, scanned. paste: its link, pasted. retry: the QR code, and if that fails,
  # a new one scanned within 30 s. newcode: a QR code shown, then Connect a device opened again (a new code),
  # and the first QR code scanned.
  link | paste | retry | newcode | three) LINKWAY=yes ;;
  *) LINKWAY= ;;
esac
if [ -n "$LINKWAY" ]; then
  kind=${PL_LINK:-qr}; [ "$WAY" = paste ] && kind=plain
  LINK=$(make_invite a "$kind")
  [ -n "$LINK" ] || die "no invite: $(cat "$OUT/invite-a.json")"
  link_step invite-made "$LINK"
  if [ "$WAY" = three ]; then
    # Three invites within 30 s (a person pressing Connect a device again and again), the last one scanned.
    for _ in 1 2; do sleep 3; LINK=$(make_invite a "$kind"); done
    link_step third-invite-made "$LINK"
  fi
  if [ "$WAY" = newcode ]; then
    cp "$OUT/invite-a.json" "$OUT/invite-a-first.json"
    sleep 2
    make_invite a "$kind" >/dev/null
    sstep new-code-made "$(grep -q '"code"' "$OUT/invite-a.json" && echo true || echo false)" "$(jstr "$(head -c 200 "$OUT/invite-a.json" | sed 's/s=[^&"]*/s=…/g')")"
  fi
  [ "$SSTATE" = restart ] && restart_server
  if [ -n "$EMU" ]; then
    echo "$LINK" >"$PHONE/link.txt.tmp"; chown "$RUNAS" "$PHONE/link.txt.tmp"; mv "$PHONE/link.txt.tmp" "$PHONE/link.txt"
    wait "$EMUPID" || RC=1
  else
    retry=()
    [ "$WAY" = retry ] && retry=(-retry "$PHONE/retry-link.txt")
    phone -mode link -link "$LINK" -state "$CSTATE" "${retry[@]}" >"$OUT/phone.log" 2>&1 &
    PPID_=$!
    if [ "$WAY" = retry ]; then
      # The second QR code, made when the phone asks for it.
      while kill -0 "$PPID_" 2>/dev/null; do
        if [ -f "$PHONE/retry-link.txt.ask" ]; then
          L2=$(make_invite a "$kind"); link_step retry-invite-made "$L2"
          echo "$L2" >"$PHONE/retry-link.txt.tmp"; chown "$RUNAS" "$PHONE/retry-link.txt.tmp"; mv "$PHONE/retry-link.txt.tmp" "$PHONE/retry-link.txt"
          break
        fi
        sleep 0.2
      done
    fi
    wait "$PPID_" || RC=1
  fi
elif [ -n "${PL_PHONE_HOME:-}" ] && [ -z "$EMU" ] && [ "${PL_ORDER:-0.6.3}" != 0.6.2 ]; then
  # At home, from 0.6.3 on: the code Connect a device shows, typed into the phone (Enter a code), and
  # answered through the web port.
  make_invite a >/dev/null
  CODE=$(jget "d.get('code') or ''" <"$OUT/invite-a.json")
  [ -n "$CODE" ] || die "Connect a device gave no code: $(head -c 300 "$OUT/invite-a.json")"
  sstep code-shown true null
  [ "$SSTATE" = restart ] && restart_server
  phone -mode webcode -home "$PL_PHONE_HOME" -code "$CODE" -state "$CSTATE" >"$OUT/phone.log" 2>&1 || RC=1
else
  # Away: the phone shows a code (Join your server from away) and the owner types it into the server.
  CODEF=$OUT/phone/code.txt
  if [ -n "$EMU" ]; then
    PPID_=$EMUPID
  else
    phone -mode code -state "$CSTATE" -codefile "$CODEF" >"$OUT/phone.log" 2>&1 &
    PPID_=$!
  fi
  for _ in $(seq 1 240); do [ -s "$CODEF" ] && break; sleep 0.5; done
  [ -s "$CODEF" ] || die "the phone showed no code"
  [ "$SSTATE" = restart ] && restart_server
  ans=$(api a POST /api/p2p/join "{\"code\":\"$(cat "$CODEF")\",\"password\":\"$PW\"}")
  sstep code-typed "$(grep -q '"error"' <<<"$ans" && echo false || echo true)" "$(jstr "$(head -c 300 <<<"$ans")")"
  wait "$PPID_" || RC=1
fi
sleep 2 # one more look at the server's side
# The phone stopped without saying why: its own last words.
if [ "$RC" != 0 ] && ! grep -qE '"side": ?"phone", ?"step": ?"result"' "$STEPS"; then
  SIDE=phone sstep result false "$(jstr "$(tail -c 400 "$OUT/phone.log")")"
fi
finish "$RC"
