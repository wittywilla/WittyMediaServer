#!/bin/sh
# Installs an unpacked WittyMediaServer archive as a system service on a Linux or FreeBSD
# machine with no package for it. Everything stays inside the install folder.
#
#   sudo ./install.sh [--prefix /opt/wittymediaserver] [--data <folder>] [--port 9797] [--user wms]
#
# --port goes in settings.ini in the data folder, where Settings → Network changes it too. Left
# out, the port stays what it was: one an earlier run of this script gave the service carries over.
# The data folder defaults to /var/lib/wittymediaserver, or on FreeBSD /var/db/wittymediaserver.
# On Debian, Ubuntu, Fedora and FreeBSD you can use the .deb, .rpm or .pkg instead. To remove it:
#   sudo /opt/wittymediaserver/bin/wittymediaserver --uninstall
set -e
PREFIX=/opt/wittymediaserver
DATA=""
PORT=""
USER_NAME=wms
while [ $# -gt 0 ]; do
  case "$1" in
    --prefix) PREFIX="$2"; shift 2 ;;
    --data) DATA="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    --user) USER_NAME="$2"; shift 2 ;;
    *) echo "unknown option $1"; exit 2 ;;
  esac
done
HERE="$(cd "$(dirname "$0")" && pwd)"
OS="$(uname -s)"
[ "$(id -u)" = 0 ] || { echo "please run as root (sudo ./install.sh)"; exit 1; }
case "$OS" in
  Linux|FreeBSD) ;;
  Darwin) echo "On a Mac, use the WittyMediaServer .pkg instead."; exit 1 ;;
  *) echo "unsupported system $OS: run $HERE/bin/wittymediaserver serve by hand"; exit 1 ;;
esac

if [ -z "$DATA" ]; then
  DATA=/var/lib/wittymediaserver
  # Where earlier versions kept it on FreeBSD stays, if it's in use.
  if [ "$OS" = FreeBSD ] && [ ! -f "$DATA/wms.db" ]; then DATA=/var/db/wittymediaserver; fi
fi
# The port an earlier run gave the service, when it was only a port.
if [ -z "$PORT" ]; then
  old=""
  case "$OS" in
    Linux)   old=$(sed -n 's/^Environment=WMS_LISTEN=//p' /etc/systemd/system/wittymediaserver.service 2>/dev/null | head -n 1) ;;
    FreeBSD) old=$(sysrc -qn wittymediaserver_listen 2>/dev/null || true)
             [ -n "$old" ] || old=$(sed -n 's/^: ..wittymediaserver_listen:=\(.*\)}$/\1/p' /usr/local/etc/rc.d/wittymediaserver 2>/dev/null | head -n 1) ;;
  esac
  case "$old" in
    :[0-9]*|0.0.0.0:[0-9]*) [ "${old##*:}" != 9797 ] && PORT="${old##*:}" ;;
  esac
fi
echo "==> installing to $PREFIX (data: $DATA)"
mkdir -p "$PREFIX"
install -d -m 0750 "$DATA" # holds password hashes and sessions; the service account gets it below
if [ "$HERE" != "$PREFIX" ]; then
  # A copy updated from an older one: the server keeps its own files where they are.
  cp -R "$HERE/bin" "$HERE/docs" "$HERE/share" "$HERE/package-info.json" "$PREFIX/"
  # A starter set of signed modules, in a package that has one.
  if [ -d "$HERE/modules" ]; then cp -R "$HERE/modules" "$PREFIX/"; fi
  cp "$HERE/install.sh" "$PREFIX/"
  # The settings file beside the program: one someone changed stays.
  if [ -f "$HERE/settings.ini" ] && [ ! -f "$PREFIX/settings.ini" ]; then cp "$HERE/settings.ini" "$PREFIX/"; fi
  [ -f "$HERE/WittyMediaServer" ] && cp "$HERE/WittyMediaServer" "$PREFIX/"
fi
chmod -R a+rX "$PREFIX"
chmod +x "$PREFIX"/bin/*

# What packages up to 0.5.2-alpha kept in the top folder.
rm -f "$PREFIX/wms" "$PREFIX/wittyplay-host" "$PREFIX/icon.png" "$PREFIX/wittymediaserver.desktop" \
      "$PREFIX/LICENSE" "$PREFIX/NOTICE" "$PREFIX/README.md" "$PREFIX/CHANGELOG.md" "$PREFIX/THIRD_PARTY_LICENSES.md"

# The app with the tray icon, in the app menu (headless servers can ignore it).
# A machine with neither folder has no app menu, and doesn't get one made for it.
if [ -f "$PREFIX/WittyMediaServer" ] && [ -f "$PREFIX/share/wittymediaserver.desktop" ]; then
  for menu in /usr/local/share/applications /usr/share/applications; do
    if [ -d "$menu" ]; then
      sed -e "s|/usr/lib/wittymediaserver|$PREFIX|g" -e "s|^Icon=.*|Icon=$PREFIX/share/wittymediaserver.png|" \
        "$PREFIX/share/wittymediaserver.desktop" > "$menu/wittymediaserver.desktop"
      break
    fi
  done
fi

case "$OS" in
  Linux)
    id "$USER_NAME" >/dev/null 2>&1 || useradd --system --home "$DATA" --shell /usr/sbin/nologin "$USER_NAME" 2>/dev/null || useradd -r -d "$DATA" -s /sbin/nologin "$USER_NAME"
    # Let the service read GPU devices for hardware transcoding when present.
    for g in video render; do getent group "$g" >/dev/null && usermod -aG "$g" "$USER_NAME" || true; done ;;
  FreeBSD)
    pw user show "$USER_NAME" >/dev/null 2>&1 || pw useradd "$USER_NAME" -d "$DATA" -s /usr/sbin/nologin -c "WittyMediaServer" ;;
esac
chown -R "$USER_NAME" "$DATA"
chmod 750 "$DATA"
# The service can't change the program's folder, so its port goes in the data folder's copy.
if [ -n "$PORT" ]; then
  "$PREFIX/bin/wittymediaserver" config port -data "$DATA" -in-data -port "$PORT"
  chown -h "$USER_NAME" "$DATA/settings.ini" # -h: a link put there is never followed
  if [ "$OS" = FreeBSD ]; then sysrc -x wittymediaserver_listen >/dev/null 2>&1 || true; fi
fi
PORT=$(sed -n 's/^[[:space:]]*port[[:space:]]*=[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$DATA/settings.ini" "$PREFIX/settings.ini" 2>/dev/null | head -n 1)
[ -n "$PORT" ] || PORT=9797

case "$OS" in
  Linux)
    cat > /etc/systemd/system/wittymediaserver.service <<EOF
[Unit]
Description=WittyMediaServer
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$USER_NAME
Environment=WMS_DATA_DIR=$DATA
Environment=WMS_NO_BROWSER=1
ExecStart=$PREFIX/bin/wittymediaserver serve
Restart=on-failure
RestartSec=3
# 3: another server already has the data folder. Starting again would only find it again.
RestartPreventExitStatus=3
NoNewPrivileges=true
ProtectSystem=full
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
ProtectClock=true
ProtectHostname=true
ProtectProc=invisible
RestrictSUIDSGID=true
RestrictRealtime=true
RemoveIPC=true
LockPersonality=true
CapabilityBoundingSet=
AmbientCapabilities=
UMask=0027
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK
# Helpers run in a box made of these: without them they lose it.
RestrictNamespaces=user pid net ipc
# Graphics cards for transcoding, and TV tuners.
DevicePolicy=closed
DeviceAllow=char-drm rw
DeviceAllow=char-DVB rw
DeviceAllow=char-video4linux rw
SystemCallArchitectures=native
SystemCallErrorNumber=EPERM
SystemCallFilter=~@mount @reboot @swap @raw-io @module @cpu-emulation
ReadWritePaths=$DATA
# Not set: MemoryDenyWriteExecute (Java and game cores that recompile need it), PrivateDevices
# (graphics cards and tuners), PrivateUsers (the video and render groups), and ProtectSystem=strict
# or ProtectHome, as saving information beside media writes into your media folders. To keep media
# read-only, use the folder's permissions, or add ReadOnlyPaths= entries here.
# The port is in Settings → Network, or $DATA/settings.ini.

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable wittymediaserver
    systemctl restart wittymediaserver
    echo "==> running: systemctl status wittymediaserver" ;;
  FreeBSD)
    mkdir -p /usr/local/etc/rc.d
    sed -e "s|%%USER%%|$USER_NAME|g" -e "s|%%DATA%%|$DATA|g" -e "s|%%PORT%%|$PORT|g" -e "s|%%PROGRAM%%|$PREFIX|g" \
      "$PREFIX/share/wittymediaserver.rc" > /usr/local/etc/rc.d/wittymediaserver
    chmod +x /usr/local/etc/rc.d/wittymediaserver
    sysrc wittymediaserver_enable=YES >/dev/null
    service wittymediaserver restart
    echo "==> running: service wittymediaserver status" ;;
esac
echo "Open http://$(hostname):$PORT/ to finish setup."
echo "To remove it: sudo $PREFIX/bin/wittymediaserver --uninstall"
