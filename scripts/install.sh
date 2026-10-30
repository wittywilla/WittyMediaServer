#!/bin/sh
# Installs WittyMediaServer as a system service on Linux (systemd), macOS
# (launchd) or FreeBSD (rc.d). Everything stays inside the install directory;
# no packages are required. Run from the extracted release folder as root:
#   sudo ./install.sh [--prefix /opt/wittymediaserver] [--data /var/lib/wittymediaserver] [--port 9797] [--user wms]
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

echo "==> installing to $PREFIX (data: $DATA, port: $PORT)"
mkdir -p "$PREFIX" "$DATA"
cp -R "$HERE"/wms "$HERE"/bin "$PREFIX"/ 2>/dev/null || cp "$HERE"/wms "$PREFIX"/
cp "$HERE"/*.md "$HERE"/LICENSE "$HERE"/NOTICE "$PREFIX"/ 2>/dev/null || true
# Tray launcher + desktop entry (optional; headless servers simply skip it).
if [ -f "$HERE/WittyMediaServer" ]; then
  cp "$HERE/WittyMediaServer" "$PREFIX/" && chmod +x "$PREFIX/WittyMediaServer"
  [ -f "$HERE/icon.png" ] && cp "$HERE/icon.png" "$PREFIX/"
  if [ -f "$HERE/wittymediaserver.desktop" ] && [ "$OS" != Darwin ]; then
    sed "s|/opt/wittymediaserver|$PREFIX|g" "$HERE/wittymediaserver.desktop" > /usr/local/share/applications/wittymediaserver.desktop 2>/dev/null \
      || { mkdir -p /usr/share/applications && sed "s|/opt/wittymediaserver|$PREFIX|g" "$HERE/wittymediaserver.desktop" > /usr/share/applications/wittymediaserver.desktop; }
  fi
fi
if [ -d "$HERE/WittyMediaServer.app" ] && [ "$OS" = Darwin ]; then
  rm -rf /Applications/WittyMediaServer.app && cp -R "$HERE/WittyMediaServer.app" /Applications/
  xattr -dr com.apple.quarantine /Applications/WittyMediaServer.app 2>/dev/null || true
fi
chmod +x "$PREFIX"/wms "$PREFIX"/bin/* 2>/dev/null || true

# Service user
case "$OS" in
  Linux)
    id "$USER_NAME" >/dev/null 2>&1 || useradd --system --home "$DATA" --shell /usr/sbin/nologin "$USER_NAME" 2>/dev/null || useradd -r -d "$DATA" -s /sbin/nologin "$USER_NAME"
    # Let the service read GPU devices for hardware transcoding when present.
    for g in video render; do getent group $g >/dev/null && usermod -aG $g "$USER_NAME" || true; done ;;
  FreeBSD)
    pw user show "$USER_NAME" >/dev/null 2>&1 || pw useradd "$USER_NAME" -d "$DATA" -s /usr/sbin/nologin -c "WittyMediaServer" ;;
  Darwin) USER_NAME="$(stat -f %Su /dev/console)";; # run as the logged-in user on macOS
esac
chown -R "$USER_NAME" "$DATA"

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
ExecStart=$PREFIX/wms serve
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
    systemctl enable --now wittymediaserver
    echo "==> running: systemctl status wittymediaserver" ;;
  Darwin)
    PLIST=/Library/LaunchDaemons/com.wittywilla.wittymediaserver.plist
    cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.wittywilla.wittymediaserver</string>
  <key>ProgramArguments</key><array><string>$PREFIX/wms</string><string>serve</string></array>
  <key>EnvironmentVariables</key><dict>
    <key>WMS_DATA_DIR</key><string>$DATA</string>
    <key>WMS_LISTEN</key><string>:$PORT</string>
  </dict>
  <key>UserName</key><string>$USER_NAME</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$DATA/wms.log</string>
  <key>StandardErrorPath</key><string>$DATA/wms.log</string>
</dict></plist>
EOF
    xattr -dr com.apple.quarantine "$PREFIX" 2>/dev/null || true
    launchctl bootout system "$PLIST" 2>/dev/null || true
    launchctl bootstrap system "$PLIST"
    echo "==> running (launchd). Log: $DATA/wms.log" ;;
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
command_args="-f -p \${pidfile} -o \${wittymediaserver_data}/wms.log /usr/bin/env WMS_DATA_DIR=\${wittymediaserver_data} WMS_LISTEN=\${wittymediaserver_listen} $PREFIX/wms serve"
start_precmd="install -o \${wittymediaserver_user} /dev/null \${pidfile}"
run_rc_command "\$1"
EOF
    chmod +x /usr/local/etc/rc.d/wittymediaserver
    sysrc wittymediaserver_enable=YES >/dev/null
    service wittymediaserver restart
    echo "==> running: service wittymediaserver status" ;;
  *) echo "unsupported OS $OS: run $PREFIX/wms serve manually"; exit 1 ;;
esac
echo "Open http://$(hostname):$PORT/ to finish setup."
echo "Desktop users: launch \"WittyMediaServer\" from the app menu for a tray icon (do not run the service and the tray at the same time on the same port)."
