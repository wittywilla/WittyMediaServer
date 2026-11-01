#!/bin/sh
# Installs an unpacked WittyMediaServer archive as a system service on a Linux or FreeBSD
# machine with no package for it. Everything stays inside the install folder.
#
#   sudo ./install.sh [--prefix /opt/wittymediaserver] [--data <folder>] [--port 9797] [--user wms]
#
# The data folder defaults to /var/lib/wittymediaserver, or on FreeBSD /var/db/wittymediaserver.
# On Debian, Ubuntu, Fedora and FreeBSD you can use the .deb, .rpm or .pkg instead. To remove it:
#   sudo /opt/wittymediaserver/bin/wittymediaserver --uninstall
set -e
PREFIX=/opt/wittymediaserver
DATA=""
PORT=9797
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
echo "==> installing to $PREFIX (data: $DATA, port: $PORT)"
mkdir -p "$PREFIX"
install -d -m 0750 "$DATA" # holds password hashes and sessions; the service account gets it below
if [ "$HERE" != "$PREFIX" ]; then
  # A copy updated from an older one: the server keeps its own files where they are.
  cp -R "$HERE/bin" "$HERE/docs" "$HERE/share" "$HERE/package-info.json" "$PREFIX/"
  cp "$HERE/install.sh" "$PREFIX/"
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
Environment=WMS_LISTEN=:$PORT
Environment=WMS_NO_BROWSER=1
ExecStart=$PREFIX/bin/wittymediaserver serve
Restart=on-failure
RestartSec=3
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
