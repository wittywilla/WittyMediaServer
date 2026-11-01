#!/usr/bin/env bash
# Proves the Linux packages for real, in throwaway containers with systemd running: install,
# use, upgrade, purge, and a snapshot of the file system from before and after to show nothing
# is left. Also takes over from a real copy that install.sh of 0.5.2-alpha installed.
#
#   scripts/test/installers-linux.sh <folder with packages> [debian:trixie-slim ubuntu:26.04 fedora:43 ...]
#
# The folder holds:
#   wittymediaserver-<new>-linux-amd64.{deb,rpm}       the packages being tested
#   upgrade/wittymediaserver-*-linux-amd64.{deb,rpm}   the same built as a later version
#   old/wittymediaserver-0.5.2-alpha-linux-amd64.tar.gz  (optional) a real old release
#   archive/wittymediaserver-*-linux-amd64.tar.gz        (optional) the plain archive
#
# Runs where Docker runs Linux containers. Writes each run's listings to <folder>/results/.
set -uo pipefail

PKGS="$(cd "${1:?folder with packages}" && pwd)"; shift
IMAGES=("$@"); [ ${#IMAGES[@]} -gt 0 ] || IMAGES=(debian:trixie-slim ubuntu:26.04 fedora:43)
RESULTS="$PKGS/results"; mkdir -p "$RESULTS"
FAILED=0

# What a machine does by itself, so isn't the package's doing.
SNAPSHOT='find / -xdev \( -path /proc -o -path /sys -o -path /dev -o -path /run -o -path /tmp -o -path /pkgs -o -path /var/log -o -path /var/cache -o -path /var/tmp \
  -o -path /var/lib/dpkg -o -path /var/lib/apt -o -path /var/lib/rpm -o -path /var/lib/dnf -o -path /usr/lib/sysimage/rpm -o -path /usr/lib/sysimage/libdnf5 \
  -o -path /var/lib/systemd -o -path /etc/ld.so.cache -o -path /usr/share/applications/mimeinfo.cache \
  -o -path /usr/share/icons/hicolor/icon-theme.cache -o -path /var/lib/private -o -path /root/.cache \) -prune -o -print 2>/dev/null | sort; \
  echo "--- accounts"; cut -d: -f1 /etc/passwd | sort; cut -d: -f1 /etc/group | sort; \
  echo "--- services"; systemctl list-unit-files --no-pager --no-legend 2>/dev/null | grep -i witty || true'

for image in "${IMAGES[@]}"; do
  slug="${image//[:\/]/-}"
  log="$RESULTS/$slug.log"
  tag="wms-installer-test-$slug"
  echo "==> $image"
  case "$image" in
    fedora*|rocky*|alma*|opensuse*) kind=rpm; prep='dnf install -y systemd procps-ng findutils hostname shadow-utils curl' ;;
    *) kind=deb; prep='apt-get update && apt-get install -y --no-install-recommends systemd systemd-sysv procps curl ca-certificates' ;;
  esac
  docker build -q -t "$tag" - >"$log" 2>&1 <<EOF
FROM $image
RUN $prep
CMD ["/sbin/init"]
EOF
  if [ $? -ne 0 ]; then echo "   FAILED to prepare the image: see $log"; FAILED=1; continue; fi

  name="wms-inst-$slug"
  docker rm -f "$name" >/dev/null 2>&1
  docker run -d --name "$name" --privileged --cgroupns=host -v /sys/fs/cgroup:/sys/fs/cgroup:rw -v "$PKGS:/pkgs:ro" --tmpfs /run --tmpfs /tmp "$tag" >>"$log" 2>&1
  for _ in $(seq 1 30); do docker exec "$name" systemctl is-system-running 2>/dev/null | grep -Eq 'running|degraded' && break; sleep 1; done

  docker exec -i -e KIND="$kind" -e SNAPSHOT="$SNAPSHOT" "$name" bash -s >>"$log" 2>&1 <<'SCRIPT'
set -u
fail() { echo "FAIL: $*"; exit 1; }
say() { echo; echo "##### $*"; }
install_pkg() {
  if [ "$KIND" = deb ]; then DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "$@" >/tmp/out 2>&1 || { cat /tmp/out; fail "installing"; }
  else dnf install -y "$@" >/tmp/out 2>&1 || { cat /tmp/out; fail "installing"; }; fi
}
wait_up() {
  for _ in $(seq 1 40); do curl -fsS "http://127.0.0.1:${1:-9797}/api/health" >/dev/null 2>&1 && return 0; sleep 1; done
  systemctl status wittymediaserver --no-pager; journalctl -u wittymediaserver --no-pager -n 40; fail "the server didn't answer on ${1:-9797}"
}
pkg=$(ls /pkgs/wittymediaserver-*-linux-amd64.$KIND)
new=$(ls /pkgs/upgrade/wittymediaserver-*-linux-amd64.$KIND)

say "snapshot before"
eval "$SNAPSHOT" > /tmp/before.txt
mkdir -p /srv/media && echo film > "/srv/media/A Made-Up Film.mkv"

say "install: $pkg"
install_pkg "$pkg"
systemctl is-enabled wittymediaserver || fail "not enabled"
wait_up
wittymediaserver version
[ "$(stat -c %U /var/lib/wittymediaserver)" = wms ] || fail "the data folder isn't the service's"
ps -o user=,args= -C wittymediaserver | grep '^wms ' || fail "not running as its own account"
ls /var/lib/wittymediaserver
mkdir -p /var/lib/wittymediaserver/recordings
echo "my file" > /var/lib/wittymediaserver/my-own-notes.txt
find /usr/lib/wittymediaserver -type f | sort > /tmp/files-installed.txt
cat /tmp/files-installed.txt

say "the package is how to update"
curl -fsS http://127.0.0.1:9797/api/health
echo

say "upgrade: $new"
stat -c '%i' /var/lib/wittymediaserver/wms.db > /tmp/db-inode
install_pkg "$new"
wait_up
wittymediaserver version | tee /tmp/v
grep -q 9.9.9 /tmp/v || fail "still the old version"
[ -f /var/lib/wittymediaserver/wms.db ] || fail "the upgrade lost the database"
[ -f /var/lib/wittymediaserver/my-own-notes.txt ] || fail "the upgrade lost a file"
systemctl is-enabled wittymediaserver || fail "not enabled after upgrade"

say "dry run changes nothing"
wittymediaserver --uninstall --dry-run
systemctl is-active wittymediaserver || fail "a dry run stopped the service"

say "remove, keeping data"
if [ "$KIND" = deb ]; then apt-get remove -y wittymediaserver >/tmp/out 2>&1 || { cat /tmp/out; fail removing; }
else dnf remove -y wittymediaserver >/tmp/out 2>&1 || { cat /tmp/out; fail removing; }; fi
tail -n 5 /tmp/out
[ ! -e /usr/lib/wittymediaserver ] || { find /usr/lib/wittymediaserver; fail "the program stayed"; }
[ -f /var/lib/wittymediaserver/wms.db ] || fail "plain remove took the library"
systemctl status wittymediaserver --no-pager >/dev/null 2>&1; [ $? -ne 0 ] || fail "the service is still running"

say "install again: the library is still there"
install_pkg "$pkg"
wait_up

say "full removal through --uninstall, asking"
printf 'y\ny\n' | wittymediaserver --uninstall | tee /tmp/uninstall.txt
grep -q 'never touched' /tmp/uninstall.txt || fail "doesn't say media is safe"
grep -q 'my-own-notes.txt' /tmp/uninstall.txt || fail "doesn't list what it left"
[ -f /var/lib/wittymediaserver/my-own-notes.txt ] || fail "removed something it didn't make"
rm -f /var/lib/wittymediaserver/my-own-notes.txt
rmdir /var/lib/wittymediaserver 2>/dev/null
[ -f "/srv/media/A Made-Up Film.mkv" ] || fail "a film was deleted"
rm -rf /srv/media

say "snapshot after"
eval "$SNAPSHOT" > /tmp/after.txt
if ! diff -u /tmp/before.txt /tmp/after.txt > /tmp/diff.txt; then
  cat /tmp/diff.txt
  fail "something was left behind or changed"
fi
echo "NOTHING LEFT: $(wc -l < /tmp/after.txt) paths before and after are the same"

if [ "$KIND" = deb ]; then
  say "apt purge removes everything too, leaving recordings"
  install_pkg "$pkg"; wait_up
  mkdir -p /var/lib/wittymediaserver/recordings && echo tv > "/var/lib/wittymediaserver/recordings/A Made-Up Show.ts"
  apt-get purge -y wittymediaserver > /tmp/out 2>&1 || { cat /tmp/out; fail purging; }
  tail -n 4 /tmp/out
  [ -f "/var/lib/wittymediaserver/recordings/A Made-Up Show.ts" ] || fail "purge took a recording"
  rm -rf /var/lib/wittymediaserver
  eval "$SNAPSHOT" > /tmp/after2.txt
  diff -u /tmp/before.txt /tmp/after2.txt || fail "purge left something behind"
  echo "NOTHING LEFT after purge"
fi

old=$(ls /pkgs/old/wittymediaserver-0.5.2-alpha-linux-amd64.tar.gz 2>/dev/null)
if [ -n "$old" ]; then
  say "taking over from a real 0.5.2-alpha, installed with its own install.sh, custom data, port and account"
  mkdir -p /tmp/old && tar -xzf "$old" -C /tmp/old --strip-components=1
  useradd --system --shell /usr/sbin/nologin media
  sh /tmp/old/install.sh --prefix /srv/wms --data /tank/wms --port 8123 --user media >/tmp/out 2>&1 || { cat /tmp/out; fail "the old install"; }
  wait_up 8123
  /srv/wms/wms version
  echo kept > /tank/wms/kept-by-upgrade.txt
  systemctl is-active wittymediaserver
  install_pkg "$pkg"
  wait_up 8123
  [ -f /tank/wms/kept-by-upgrade.txt ] || fail "the data folder was lost"
  [ ! -e /srv/wms/wms ] || fail "the old program stayed"
  [ ! -e /etc/systemd/system/wittymediaserver.service ] || fail "the old service stayed"
  cat /etc/systemd/system/wittymediaserver.service.d/upgraded-from-install-sh.conf
  ps -o user=,args= -C wittymediaserver | grep -E '^media ' || fail "the account wasn't carried over"
  wittymediaserver version
  echo "TOOK OVER, kept data, port and account"
  printf 'y\ny\n' | wittymediaserver --uninstall > /tmp/uninstall2.txt; cat /tmp/uninstall2.txt
  [ ! -e /tank/wms/wms.db ] || fail "the custom data folder's database stayed"
fi
arc=$(ls /pkgs/archive/wittymediaserver-*-linux-amd64.tar.gz 2>/dev/null)
if [ -n "$arc" ]; then
  # The account and home folders the takeover test made aren't this test's.
  id media >/dev/null 2>&1 && userdel media
  rm -rf /tank /srv/wms /tmp/old
  eval "$SNAPSHOT" > /tmp/before3.txt

  say "the plain archive, with its install.sh, then --uninstall"
  mkdir -p /tmp/arc && tar -xzf "$arc" -C /tmp/arc --strip-components=1
  sh /tmp/arc/install.sh --prefix /srv/wittymedia --data /srv/wittydata --port 8124 > /tmp/out 2>&1 || { cat /tmp/out; fail "install.sh"; }
  wait_up 8124
  ls /srv/wittymedia /srv/wittymedia/bin
  [ ! -e /srv/wittymedia/wms ] || fail "install.sh brought the forwarder"
  /srv/wittymedia/bin/wittymediaserver --uninstall --yes --remove-data > /tmp/out 2>&1; code=$?
  cat /tmp/out
  [ $code -eq 0 ] || fail "uninstall exit $code"
  [ ! -e /srv/wittymedia ] || { find /srv/wittymedia; fail "the program stayed"; }
  [ ! -e /srv/wittydata ] || { find /srv/wittydata; fail "the data stayed"; }
  eval "$SNAPSHOT" > /tmp/after3.txt
  diff -u /tmp/before3.txt /tmp/after3.txt || fail "the archive's install left something behind"
  echo "NOTHING LEFT after install.sh and --uninstall"

  say "a real unpacked 0.5.2-alpha, portable, whose update lands the new package over it"
  rm -rf /srv/p && mkdir -p /srv/p/data
  tar -xzf "$old" -C /srv/p --strip-components=1
  echo '{"portable":true,"dataDir":"data","listen":":8125"}' > /srv/p/wms.json
  (cd /srv/p && WMS_NO_BROWSER=1 nohup ./wms serve > /tmp/p.log 2>&1 & echo $! > /tmp/p.pid)
  wait_up 8125 || cat /tmp/p.log
  [ -f /srv/p/data/wms.db ] || fail "the old copy isn't portable"
  kill "$(cat /tmp/p.pid)"; sleep 2
  # What 0.5.2's updater does: every file in the package goes in, nothing else is touched.
  tar -xzf "$arc" -C /srv/p --strip-components=1
  (cd /srv/p && WMS_NO_BROWSER=1 nohup ./wms serve > /tmp/p.log 2>&1 & echo $! > /tmp/p.pid)
  wait_up 8125
  /srv/p/wms version | tee /tmp/v; grep -q "^wittymediaserver " /tmp/v || fail "./wms isn't the new server"
  [ -f /srv/p/data/wms.db ] || fail "the update lost the library"
  sleep 2
  ls -a /srv/p
  for f in wittyplay-host LICENSE README.md icon.png; do [ ! -e "/srv/p/$f" ] || fail "$f, from the old package, stayed"; done
  pkill -f /srv/p/bin/wittymediaserver; kill "$(cat /tmp/p.pid)" 2>/dev/null
  rm -rf /srv/p
  echo "AN UNPACKED 0.5.2 TOOK THE UPDATE, kept its library, and the old command still works"
fi
echo ALLPASSED
SCRIPT
  if docker exec "$name" true 2>/dev/null && tail -n 1 "$log" | grep -q ALLPASSED; then
    echo "   passed: $log"
  else
    echo "   FAILED: $log"
    tail -n 25 "$log"
    FAILED=1
  fi
  docker rm -f "$name" >/dev/null 2>&1
done
exit $FAILED
