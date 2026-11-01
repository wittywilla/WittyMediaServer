#!/bin/sh
# Installs an unpacked WittyMediaServer archive as a system service on a Linux or FreeBSD
# machine with no package for it. Everything stays inside the install folder.
#
#   sudo ./install.sh [--prefix /opt/wittymediaserver] [--data /var/lib/wittymediaserver] [--port 9797] [--user wms]
#
# On Debian, Ubuntu, Fedora and their relatives use the .deb or .rpm instead. To remove it:
#   sudo /opt/wittymediaserver/bin/wittymediaserver --uninstall
set -e
PREFIX=/opt/wittymediaserver
DATA=/var/lib/wittymediaserver
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

echo "==> installing to $PREFIX (data: $DATA, port: $PORT)"
mkdir -p "$PREFIX" "$DATA"
if [ "$HERE" != "$PREFIX" ]; then
  # A copy updated from an older one: the server keeps its own files where they are.
  cp -R "$HERE/bin" "$HERE/docs" "$HERE/package-info.json" "$PREFIX/"
  cp "$HERE/install.sh" "$PREFIX/"
  [ -f "$HERE/WittyMediaServer" ] && cp "$HERE/WittyMediaServer" "$PREFIX/"
fi
chmod -R a+rX "$PREFIX"
chmod +x "$PREFIX"/bin/*

# What packages up to 0.5.2-alpha kept in the top folder.
rm -f "$PREFIX/wms" "$PREFIX/wittyplay-host" "$PREFIX/icon.png" "$PREFIX/wittymediaserver.desktop" \
      "$PREFIX/LICENSE" "$PREFIX/NOTICE" "$PREFIX/README.md" "$PREFIX/CHANGELOG.md" "$PREFIX/THIRD_PARTY_LICENSES.md"

# The app with the tray icon, in the app menu (headless servers can ignore it).
if [ -f "$PREFIX/WittyMediaServer" ] && [ -f "$PREFIX/bin/wittymediaserver.desktop" ] && [ "$OS" = Linux ]; then
  mkdir -p /usr/local/share/applications
  sed -e "s|/usr/lib/wittymediaserver|$PREFIX|g" -e "s|^Icon=.*|Icon=$PREFIX/bin/wittymediaserver.png|" \
    "$PREFIX/bin/wittymediaserver.desktop" > /usr/local/share/applications/wittymediaserver.desktop
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
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
ReadWritePaths=$DATA
# Media folders are read-only by default; add ReadWritePaths= entries if you need deletes.

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable wittymediaserver
    systemctl restart wittymediaserver
    echo "==> running: systemctl status wittymediaserver" ;;
  FreeBSD)
    cat > /usr/local/etc/rc.d/wittymediaserver <<EOF
#!/bin/sh
# PROVIDE: wittymediaserver
# REQUIRE: NETWORKING
# KEYWORD: shutdown
. /etc/rc.subr
name=wittymediaserver
rcvar=wittymediaserver_enable
load_rc_config \$name
: \${wittymediaserver_enable:=NO}
: \${wittymediaserver_user:=$USER_NAME}
: \${wittymediaserver_data:=$DATA}
: \${wittymediaserver_listen:=:$PORT}
pidfile=/var/run/\${name}.pid
command=/usr/sbin/daemon
command_args="-f -p \${pidfile} -o \${wittymediaserver_data}/wms.log /usr/bin/env WMS_NO_BROWSER=1 WMS_DATA_DIR=\${wittymediaserver_data} WMS_LISTEN=\${wittymediaserver_listen} $PREFIX/bin/wittymediaserver serve"
start_precmd="install -o \${wittymediaserver_user} /dev/null \${pidfile}"
run_rc_command "\$1"
EOF
    chmod +x /usr/local/etc/rc.d/wittymediaserver
    sysrc wittymediaserver_enable=YES >/dev/null
    service wittymediaserver restart
    echo "==> running: service wittymediaserver status" ;;
esac
echo "Open http://$(hostname):$PORT/ to finish setup."
echo "To remove it: sudo $PREFIX/bin/wittymediaserver --uninstall"
