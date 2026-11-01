#!/usr/bin/env bash
# Proves the MSI by really running it, in a throwaway container with Wine: install for one person
# and for everyone, upgrade, uninstall, and the files and registry from before and after, to show
# nothing is left. Wine isn't Windows, so installers-windows-sandbox.wsb has the last word, but
# Windows Installer's tables, the uninstaller and the registry are all exercised here.
#
#   scripts/test/installers-wine.sh <folder with the .msi files> [screenshots]
#
# The folder holds wittymediaserver-*-windows-amd64.msi, and upgrade/ the same at a later version.
# Writes its listing to <folder>/results/wine.log. With "screenshots" it also photographs the wizard.
set -uo pipefail

PKGS="$(cd "${1:?folder with the msi}" && pwd)"
SHOTS="${2:-}"
RESULTS="$PKGS/results"; mkdir -p "$RESULTS"
log="$RESULTS/wine.log"
tag=wms-installer-test-wine

docker build -q -t "$tag" - >"$log" 2>&1 <<'EOF'
FROM debian:trixie-slim
RUN dpkg --add-architecture i386 && apt-get update && apt-get install -y --no-install-recommends \
    wine wine64 wine32:i386 xvfb xauth x11-utils imagemagick xdotool procps file fonts-liberation
EOF
[ $? -eq 0 ] || { echo "FAILED to prepare the image: see $log"; tail -n 20 "$log"; exit 1; }

docker rm -f wms-inst-wine >/dev/null 2>&1
docker run -i --rm --name wms-inst-wine -e SHOTS="$SHOTS" -v "$PKGS:/pkgs" "$tag" bash -s >>"$log" 2>&1 <<'SCRIPT'
set -u
export WINEPREFIX=/root/wine WINEARCH=win64 WINEDEBUG=-all DISPLAY=:99
Xvfb :99 -screen 0 1024x768x24 >/dev/null 2>&1 &
sleep 1
fail() { echo "FAIL: $*"; exit 1; }
say() { echo; echo "##### $*"; }
wineboot -i >/dev/null 2>&1
wineserver -w
msi=$(ls /pkgs/wittymediaserver-*-windows-amd64.msi)
new=$(ls /pkgs/upgrade/wittymediaserver-*-windows-amd64.msi)
C="$WINEPREFIX/drive_c"
USERDIR="$C/users/root"

snapshot() {
  wineserver -w
  # Wine's own bookkeeping isn't the installer's doing.
  (cd "$C" && find . -path ./windows/Installer -prune -o -path ./windows/logs -prune -o -path ./windows/temp -prune \
      -o -path ./users/root/Temp -prune -o -path ./users/root/AppData/Local/Temp -prune -o -print | sort)
  echo "--- registry"
  for f in system.reg user.reg; do
    sed -E 's/^(\[[^]]*\]).*/\1/' "$WINEPREFIX/$f" | grep -Eai 'witty|^"[^"]*witty' | sort
  done
}
reg_has() { grep -aqi "$1" "$WINEPREFIX/system.reg" "$WINEPREFIX/user.reg"; }

run_msi() {
  wine msiexec "$@" /l*v 'C:\msi.log'
  code=$?
  wineserver -w
  return $code
}

say "snapshot before"
snapshot > /tmp/before.txt
wc -l /tmp/before.txt

if [ -n "${SHOTS:-}" ]; then
  say "photographing the wizard"
  mkdir -p /pkgs/results/wizard
  wine msiexec /i "$msi" >/dev/null 2>&1 &
  sleep 25
  shoot() {
    import -window root "/pkgs/results/wizard/$1.png" 2>/dev/null
  }
  shoot 1-welcome
  xdotool key Return; sleep 3; shoot 2-who-is-it-for
  xdotool key Return; sleep 3; shoot 3-how-should-it-run-just-me
  xdotool key alt+b; sleep 2
  xdotool key Down; sleep 1; xdotool key Return; sleep 3; shoot 4-how-should-it-run-everyone
  xdotool key Return; sleep 3; shoot 5-ready
  pkill -f msiexec
  wineserver -k; sleep 2
fi

say "install just for me, silently, with choices: $msi"
run_msi /i "$msi" /qn MSIINSTALLPERUSER=1 AUTOSTART=1 DESKTOPSHORTCUT=1 ADDTOPATH=1 || fail "install exit $code"
snapshot > /tmp/installed.txt
diff /tmp/before.txt /tmp/installed.txt | grep '^>' | head -60
dir=$(find "$C" -type f -name WittyMediaServer.exe | head -1)
[ -n "$dir" ] || fail "the app wasn't installed"
root=$(dirname "$dir")
echo "installed to: $root"
for f in "Uninstall WittyMediaServer.exe" package-info.json bin/wittymediaserver.exe docs/LICENSE; do
  [ -f "$root/$f" ] || fail "$f is missing"
done
[ ! -e "$root/wms.exe" ] || fail "the installer brought the forwarder"
grep -q '"msi"' "$root/package-info.json" || fail "the marker doesn't say msi"
reg_has 'Uninstall\\\\\\\\WittyMediaServer' || fail "no Installed apps entry"
reg_has 'ProductCode' || fail "no product code in the entry"

say "the server starts from where it was installed"
wine "$root/bin/wittymediaserver.exe" version
wine "$root/bin/wittymediaserver.exe" config path

say "upgrade in place: $new"
echo mine > "$root/my-own-file.txt"
run_msi /i "$new" /qn || fail "upgrade exit $code"
grep -a 'Product: WittyMediaServer' /root/wine/drive_c/msi.log | tail -n 3
wine "$root/bin/wittymediaserver.exe" version | tee /tmp/v
grep -q 9.9.9 /tmp/v || fail "still the old version"
[ -f "$root/my-own-file.txt" ] || fail "the upgrade took a file it didn't make"
wineserver -w

say "the data a real server leaves"
data="$USERDIR/AppData/Local/WittyMediaServer"
mkdir -p "$data/logs" "$data/cache/images" "$data/modules/wittyplay" "$data/gaming/cores" "$data/recordings"
echo x > "$data/logs/wms.log"; echo x > "$data/cache/images/a.jpg"; echo x > "$data/modules/wittyplay/x"
echo x > "$data/gaming/cores/c.dll"
mkdir -p "$USERDIR/AppData/Roaming/WittyMediaServer"; echo '{}' > "$USERDIR/AppData/Roaming/WittyMediaServer/wms.json"
echo notes > "$data/my-own-notes.txt"
echo tv > "$data/recordings/A Made-Up Show.ts"
mkdir -p "$USERDIR/Videos"; echo film > "$USERDIR/Videos/A Made-Up Film.mkv"

say "uninstall silently, keeping data"
wine "$root/Uninstall WittyMediaServer.exe" /S
wineserver -w; sleep 8; wineserver -w
[ ! -e "$root/bin/wittymediaserver.exe" ] || fail "the program stayed"
[ -f "$data/cache/images/a.jpg" ] || fail "a keep-data uninstall took the cache"
[ -f "$root/my-own-file.txt" ] || fail "took a file it didn't make"
reg_has 'Uninstall\\\\\\\\WittyMediaServer' && fail "the Installed apps entry stayed"
rm -f "$root/my-own-file.txt"; rmdir "$root" 2>/dev/null

say "install again, for full removal"
run_msi /i "$msi" /qn MSIINSTALLPERUSER=1 || fail "reinstall exit $code"
root=$(dirname "$(find "$C" -type f -name WittyMediaServer.exe | head -1)")
report='C:\report.txt'

say "uninstall silently, removing everything"
wine "$root/Uninstall WittyMediaServer.exe" /S /RemoveData --report "$report"
wineserver -w; sleep 8; wineserver -w
echo "--- what it says it did"; cat "$C/report.txt" || fail "no report"
grep -q 'A Made-Up Show\|recordings' "$C/report.txt" || fail "the recordings aren't reported"
[ -f "$data/recordings/A Made-Up Show.ts" ] || fail "took a recording without being asked"
[ -f "$data/my-own-notes.txt" ] || fail "took a file it didn't make"
[ -f "$USERDIR/Videos/A Made-Up Film.mkv" ] || fail "A FILM WAS DELETED"
rm -f "$data/my-own-notes.txt" "$data/recordings/A Made-Up Show.ts" "$USERDIR/Videos/A Made-Up Film.mkv" "$C/report.txt"
rmdir "$data/recordings" "$data" 2>/dev/null

say "snapshot after"
snapshot > /tmp/after.txt
diff -u /tmp/before.txt /tmp/after.txt | grep -v msi.log > /tmp/d.txt
if grep -Eq '^[+-][^+-]' /tmp/d.txt; then cat /tmp/d.txt; fail "something was left behind"; fi
echo "NOTHING LEFT: $(wc -l < /tmp/after.txt) paths and registry keys, before and after the same"

say "for everyone, with the service and the firewall rule"
run_msi /i "$msi" /qn 'MSIINSTALLPERUSER=' INSTALLSERVICE=1 ADDFIREWALL=1 AUTOSTART= PORT=9797 || fail "machine install exit $code"
root="$C/Program Files/WittyMediaServer"
[ -f "$root/WittyMediaServer.exe" ] || fail "not in Program Files: $(find "$C" -name WittyMediaServer.exe)"
grep -aqi 'Services\\\\\\\\WittyMediaServer' "$WINEPREFIX/system.reg" || fail "the service wasn't registered"
grep -a -A6 'Services\\\\\\\\WittyMediaServer\]' "$WINEPREFIX/system.reg" | head -12
grep -aq 'serve -data' "$WINEPREFIX/system.reg" || fail "the service has no arguments"
wine "$root/Uninstall WittyMediaServer.exe" /S /RemoveData
wineserver -w; sleep 8; wineserver -w
[ ! -e "$root/bin/wittymediaserver.exe" ] || fail "the program stayed in Program Files"
grep -aqi 'Services\\\\\\\\WittyMediaServer' "$WINEPREFIX/system.reg" && fail "the service stayed"
rm -rf "$C/ProgramData/WittyMediaServer"
snapshot > /tmp/after2.txt
diff -u /tmp/before.txt /tmp/after2.txt | grep -v msi.log > /tmp/d2.txt
if grep -Eq '^[+-][^+-]' /tmp/d2.txt; then cat /tmp/d2.txt; fail "something was left behind by the for-everyone install"; fi
echo "NOTHING LEFT after the for-everyone install either"
echo ALLPASSED
SCRIPT
if tail -n 1 "$log" | grep -q ALLPASSED; then
  echo "passed: $log"
else
  echo "FAILED: $log"
  tail -n 40 "$log"
  exit 1
fi
