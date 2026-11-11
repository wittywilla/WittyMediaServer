#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Proves FreeBSD's package and install.sh for real, in a throwaway FreeBSD virtual machine: rc.d
# service, upgrade, repair, removal, a before and after listing that must match, and taking over from
# a real 0.5.2-alpha. Runs where QEMU can use KVM: apt install qemu-system-x86 sshpass.
#
#   scripts/test/installers-freebsd.sh <folder> [14.4]
#
# The folder holds:
#   wittymediaserver-*-freebsd-amd64.tar.gz            the archive being tested
#   upgrade/wittymediaserver-*-freebsd-amd64.tar.gz    the same built as a later version
#   old/wittymediaserver-0.5.2-alpha-freebsd-amd64.tar.gz  (optional) a real old release
# Writes its listing to <folder>/results/freebsd-<release>.log. The image is kept in ~/.cache.
set -uo pipefail

PKGS="$(cd "${1:?folder}" && pwd)"
REL="${2:-14.4}"
SRC="$(cd "$(dirname "$0")/../.." && pwd)"
RESULTS="$PKGS/results"; mkdir -p "$RESULTS"
log="$RESULTS/freebsd-$REL.log"; : > "$log"
CACHE="$HOME/.cache/wittymediaserver-freebsd"; mkdir -p "$CACHE"
RAW="$CACHE/FreeBSD-$REL-RELEASE-amd64-BASIC-CI.raw"
if [ ! -f "$RAW" ]; then
  BASE="https://download.freebsd.org/releases/CI-IMAGES/$REL-RELEASE/amd64/Latest/FreeBSD-$REL-RELEASE-amd64-BASIC-CI"
  curl -fsSL -o "$RAW.xz" "$BASE.raw.xz" || curl -fsSL -o "$RAW.xz" "$BASE-ufs.raw.xz" || { echo "can't download the image"; exit 1; }
  xz -d "$RAW.xz"
fi
DISK="$(mktemp -d)/disk.qcow2"
qemu-img create -q -f qcow2 -F raw -b "$RAW" "$DISK" 20G
PORT=$((22000 + RANDOM % 1000))
QEMU="qemu-system-x86_64"; [ -w /dev/kvm ] || QEMU="sudo qemu-system-x86_64"
$QEMU -enable-kvm -cpu host -smp 4 -m 4096 -nographic -serial null -monitor none \
  -drive "file=$DISK,if=virtio,format=qcow2" -netdev "user,id=n,hostfwd=tcp:127.0.0.1:$PORT-:22" -device virtio-net-pci,netdev=n \
  >/dev/null 2>&1 &
VM=$!
finish() { kill "$VM" 2>/dev/null; sudo kill "$VM" 2>/dev/null; pkill -f "hostfwd=tcp:127.0.0.1:$PORT" 2>/dev/null; sudo pkill -f "hostfwd=tcp:127.0.0.1:$PORT" 2>/dev/null; rm -rf "$(dirname "$DISK")"; }
trap finish EXIT
SSH=(sshpass -p '' ssh -p "$PORT" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ServerAliveInterval=15 root@127.0.0.1)
SCP=(sshpass -p '' scp -P "$PORT" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR)
for _ in $(seq 1 90); do "${SSH[@]}" true 2>/dev/null && break; sleep 3; done
"${SSH[@]}" 'freebsd-version; uname -m' | tee -a "$log" || { echo "the machine didn't come up"; exit 1; }

tar -C "$SRC" -cf - packaging | "${SSH[@]}" 'mkdir -p /root/src && tar -xf - -C /root/src'
"${SCP[@]}" "$PKGS"/wittymediaserver-*-freebsd-amd64.tar.gz "root@127.0.0.1:/root/new.tar.gz"
"${SCP[@]}" "$PKGS"/upgrade/wittymediaserver-*-freebsd-amd64.tar.gz "root@127.0.0.1:/root/up.tar.gz"
ls "$PKGS"/old/wittymediaserver-0.5.2-alpha-freebsd-amd64.tar.gz >/dev/null 2>&1 && \
  "${SCP[@]}" "$PKGS"/old/wittymediaserver-0.5.2-alpha-freebsd-amd64.tar.gz "root@127.0.0.1:/root/old.tar.gz"

"${SSH[@]}" sh -s >>"$log" 2>&1 <<'SCRIPT'
fail() { echo "FAIL: $*"; exit 1; }
say() { echo; echo "##### $*"; }
export ASSUME_ALWAYS_YES=yes IGNORE_OSVERSION=yes
# What a machine does by itself, so isn't the package's doing.
snapshot() {
  find / -xdev \( -path /proc -o -path /dev -o -path /tmp -o -path /root -o -path /var/log -o -path /var/run -o -path /var/db/pkg -o -path /var/cache -o -path /var/tmp \
    -o -path /var/db/ntp -o -path /var/db/entropy -o -path /var/db/services.db -o -path /var/backups -o -path /var/spool -o -path /usr/local/share/applications/mimeinfo.cache \) -prune -o -print 2>/dev/null | sort
  echo "--- accounts"; cut -d: -f1 /etc/passwd | sort; cut -d: -f1 /etc/group | sort
  echo "--- rc.conf"; grep -i witty /etc/rc.conf
}
up() {
  i=0; while [ $i -lt 40 ]; do fetch -qo - "http://127.0.0.1:${1:-9797}/api/health" >/dev/null 2>&1 && return 0; i=$((i+1)); sleep 1; done
  service wittymediaserver onestatus; tail -n 20 "${2:-/var/db/wittymediaserver}/wms.log"; fail "the server didn't answer on ${1:-9797}"
}

say "getting ready: FFmpeg comes from FreeBSD's packages in this test"
ifconfig | grep -E 'flags=|inet ' | head -6; cat /etc/resolv.conf
if ! ping -c1 -t5 pkg.freebsd.org >/dev/null 2>&1; then dhclient vtnet0 >/dev/null 2>&1; fi
pkg bootstrap -y 2>&1 | tail -n 3
pkg install -y ffmpeg > /tmp/pkg.txt 2>&1 || { tail -n 15 /tmp/pkg.txt; fail "no ffmpeg"; }
ffmpeg -version | head -1
pkg info -e curl || true
mkdir -p /root/stage /root/stage2 /root/out /root/out2
tar -xzf /root/new.tar.gz -C /root/stage --strip-components=1
tar -xzf /root/up.tar.gz -C /root/stage2 --strip-components=1
ls /root/stage /root/stage/bin
/root/stage/bin/wittymediaserver version || fail "the server doesn't run here"
V1=$(/root/stage/bin/wittymediaserver version | cut -d' ' -f2)
V2=$(/root/stage2/bin/wittymediaserver version | cut -d' ' -f2)

say "making the packages"
cd /root/src
sh packaging/freebsd/build-pkg.sh "$V1" /root/stage amd64 /root/out || fail "making the package"
sh packaging/freebsd/build-pkg.sh "$V2" /root/stage2 amd64 /root/out2 || fail "making the later package"
PKG=$(ls /root/out/*.pkg); PKG2=$(ls /root/out2/*.pkg)
ls -la "$PKG" "$PKG2"
echo "versions compare: $(pkg version -t "$(pkg query -F "$PKG" %v)" "$(pkg query -F "$PKG2" %v)") (< means upgrade)"
pkg query -F "$PKG" '%n %v %q'

say "snapshot before"
mkdir -p /srv/media; echo film > "/srv/media/A Made-Up Film.mkv"
snapshot > /tmp/before.txt; wc -l /tmp/before.txt

say "install: $PKG"
pkg add "$PKG" || fail "pkg add"
pkg info -e wittymediaserver || fail "pkg doesn't know it"
[ -x /usr/local/bin/wittymediaserver ] || fail "no command"
[ -d /var/db/wittymediaserver ] || fail "no data folder"
[ "$(stat -f %Su /var/db/wittymediaserver)" = wms ] || fail "the data folder isn't the service's"
service wittymediaserver enabled && fail "it enabled itself"
sysrc wittymediaserver_enable=YES
service wittymediaserver start || fail "service start"
up
service wittymediaserver status
ps -axww -o user= -o command= | grep -i wittymediaserver | grep -v grep | cut -c1-150
ps -axww -o user= -o command= | grep 'bin/wittymediaserver serve' | grep -v grep | grep -v '^.* daemon:' | grep '^wms ' || fail "not running as its own account"
wittymediaserver version
ls /var/db/wittymediaserver

say "repair works here"
wittymediaserver doctor --json > /tmp/doctor.txt 2>&1; echo "doctor exit $?"
head -c 300 /tmp/doctor.txt; echo
wittymediaserver config path | grep '^data:' | grep -q /var/db/wittymediaserver || fail "root's command line doesn't look at the service's library"
[ ! -e /root/.local/share/wittymediaserver ] || fail "root's command line made a library of its own"

say "restarts if it stops"
pkill -f 'bin/wittymediaserver serve'; sleep 6; up

say "upgrade: $PKG2"
echo mine > /var/db/wittymediaserver/my-own-notes.txt
pkg add -f "$PKG2" || fail "the upgrade"
service wittymediaserver start; up
wittymediaserver version | tee /tmp/v; grep -q "$V2" /tmp/v || fail "still the old version"
[ -f /var/db/wittymediaserver/wms.db ] || fail "the upgrade lost the database"
[ -f /var/db/wittymediaserver/my-own-notes.txt ] || fail "the upgrade lost a file"
service wittymediaserver status

say "dry run changes nothing"
wittymediaserver --uninstall --dry-run
service wittymediaserver status || fail "a dry run stopped the service"

say "pkg delete keeps data"
pkg delete -y wittymediaserver || fail "pkg delete"
[ ! -e /usr/local/lib/wittymediaserver ] || { find /usr/local/lib/wittymediaserver; fail "the program stayed"; }
[ -f /var/db/wittymediaserver/wms.db ] || fail "plain delete took the library"
fetch -qo - http://127.0.0.1:9797/api/health >/dev/null 2>&1 && fail "still running after delete"

say "install again: the library is still there"
pkg add "$PKG" || fail "pkg add again"
service wittymediaserver start; up

say "full removal through --uninstall, asking"
printf 'y\ny\n' | wittymediaserver --uninstall | tee /tmp/un.txt
grep -q 'never touched' /tmp/un.txt || fail "doesn't say media is safe"
grep -q 'my-own-notes.txt' /tmp/un.txt || fail "doesn't list what it left"
[ -f /var/db/wittymediaserver/my-own-notes.txt ] || fail "removed something it didn't make"
[ -f "/srv/media/A Made-Up Film.mkv" ] || fail "a film was deleted"
rm -f /var/db/wittymediaserver/my-own-notes.txt; rmdir /var/db/wittymediaserver
snapshot > /tmp/after.txt
diff -u /tmp/before.txt /tmp/after.txt || fail "something was left behind"
echo "NOTHING LEFT: $(wc -l < /tmp/after.txt) paths before and after are the same"

say "the plain archive with install.sh: its own folder, data, port and account, then --uninstall"
pw useradd media -d /nonexistent -s /usr/sbin/nologin
snapshot > /tmp/before2.txt
sh /root/stage/install.sh --prefix /srv/wittymedia --data /srv/wittydata --port 8124 --user media || fail "install.sh"
up 8124 /srv/wittydata
service wittymediaserver status
ls /srv/wittymedia /srv/wittymedia/bin
[ ! -e /srv/wittymedia/wms ] || fail "install.sh brought the forwarder"
/srv/wittymedia/bin/wittymediaserver --uninstall --yes --remove-data > /tmp/un2.txt 2>&1; code=$?
cat /tmp/un2.txt
[ $code -eq 0 ] || fail "uninstall exit $code"
[ ! -e /srv/wittymedia ] || { find /srv/wittymedia; fail "the program stayed"; }
[ ! -e /srv/wittydata ] || { find /srv/wittydata; fail "the data stayed"; }
snapshot > /tmp/after2.txt
diff -u /tmp/before2.txt /tmp/after2.txt || fail "install.sh's copy left something behind"
echo "NOTHING LEFT after install.sh and --uninstall"
pw userdel media

if [ -f /root/old.tar.gz ]; then
  say "taking over from a real 0.5.2-alpha, installed with its own install.sh: custom data and port"
  rm -rf /root/old; mkdir -p /root/old; tar -xzf /root/old.tar.gz -C /root/old --strip-components=1
  # The old script needed these already there.
  mkdir -p /var/lib /usr/local/etc/rc.d /usr/local/share/applications
  sh /root/old/install.sh --prefix /opt/oldwms --data /srv/olddata --port 8125 > /tmp/out.txt 2>&1 || { cat /tmp/out.txt; fail "the old install"; }
  up 8125 /srv/olddata
  /opt/oldwms/wms version
  echo kept > /srv/olddata/kept-by-upgrade.txt
  pkg add "$PKG" || fail "the package over the old copy"
  sysrc wittymediaserver_enable wittymediaserver_data wittymediaserver_listen wittymediaserver_user
  service wittymediaserver start
  up 8125 /srv/olddata
  [ -f /srv/olddata/kept-by-upgrade.txt ] || fail "the data folder was lost"
  [ ! -e /opt/oldwms/wms ] || fail "the old program stayed"
  grep -q '/usr/local/lib/wittymediaserver/bin/wittymediaserver' /usr/local/etc/rc.d/wittymediaserver || fail "the old service stayed"
  wittymediaserver version
  echo "TOOK OVER, kept data and port"
  printf 'y\ny\n' | wittymediaserver --uninstall > /tmp/un3.txt; cat /tmp/un3.txt
  [ ! -e /srv/olddata/wms.db ] || fail "the custom data folder's database stayed"
  [ ! -e /usr/local/etc/rc.d/wittymediaserver ] || fail "the service stayed"
  grep -i witty /etc/rc.conf && fail "rc.conf still names it"
fi
echo ALLPASSED
SCRIPT
if tail -n 1 "$log" | grep -q ALLPASSED; then
  echo "passed: $log"
else
  echo "FAILED: $log"
  tail -n 40 "$log"
  exit 1
fi
