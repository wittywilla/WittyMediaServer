#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# After the files go in: the data folder, and the service running.
# deb: $1 is "configure". rpm: $1 is 1 on a first install, 2 on an upgrade.
set -e

DATA=/var/lib/wittymediaserver
RUN_AS=wms
OVERRIDE=/etc/systemd/system/wittymediaserver.service.d/upgraded-from-install-sh.conf
if [ -f "$OVERRIDE" ]; then
  u=$(sed -n 's/^User=//p' "$OVERRIDE" | head -n 1)
  [ -n "$u" ] && RUN_AS="$u"
fi

install -d -m 0750 -o "$RUN_AS" "$DATA"
# Hardware transcoding reads the GPU devices.
for g in video render; do
  if getent group "$g" >/dev/null 2>&1; then usermod -aG "$g" "$RUN_AS" 2>/dev/null || true; fi
done

# The port is chosen in Settings → Network and settings.ini now. One the service had from
# WMS_LISTEN, in a drop-in that an earlier install left or systemctl edit made, moves into the
# data folder's settings.ini, where both can change it. An address with a host in front stays.
DROP_INS=/etc/systemd/system/wittymediaserver.service.d
SERVICE_DATA=$DATA
for f in "$DROP_INS"/*.conf; do
  [ -f "$f" ] || continue
  d=$(sed -n 's/^Environment="\{0,1\}WMS_DATA_DIR=\([^"]*\)"\{0,1\}[[:space:]]*$/\1/p' "$f" | tail -n 1)
  if [ -n "$d" ]; then SERVICE_DATA="$d"; fi
done
LISTEN_LINE='^\(Environment="\{0,1\}WMS_LISTEN=\(0\.0\.0\.0\)\{0,1\}:\([0-9][0-9]*\)"\{0,1\}[[:space:]]*\)$'
for f in "$DROP_INS"/*.conf; do
  [ -f "$f" ] || continue
  port=$(sed -n "s/$LISTEN_LINE/\\3/p" "$f" | tail -n 1)
  [ -n "$port" ] || continue
  if /usr/lib/wittymediaserver/bin/wittymediaserver config port -data "$SERVICE_DATA" -in-data -port "$port"; then
    chown -h "$RUN_AS" "$SERVICE_DATA/settings.ini" 2>/dev/null || true # -h: a link put there is never followed
    sed -i "s|$LISTEN_LINE|# Moved to $SERVICE_DATA/settings.ini, where Settings, Network changes it: \\1|" "$f"
  fi
done
PORT=$(sed -n 's/^[[:space:]]*port[[:space:]]*=[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$SERVICE_DATA/settings.ini" 2>/dev/null | head -n 1)
[ -n "$PORT" ] || PORT=9797

if command -v update-desktop-database >/dev/null 2>&1; then update-desktop-database -q /usr/share/applications 2>/dev/null || true; fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then gtk-update-icon-cache -q /usr/share/icons/hicolor 2>/dev/null || true; fi

# Containers and chroots may have no systemd running. The files are still right.
if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
  if systemctl is-enabled wittymediaserver >/dev/null 2>&1 && systemctl is-active wittymediaserver >/dev/null 2>&1; then
    systemctl restart wittymediaserver
  else
    systemctl enable wittymediaserver >/dev/null 2>&1 || true
    systemctl start wittymediaserver
  fi
fi
cat <<EOF

WittyMediaServer is installed. Open http://$(hostname 2>/dev/null || echo localhost):$PORT/ to finish setup.
Status:  systemctl status wittymediaserver
Remove:  sudo wittymediaserver --uninstall (asks about your data)

EOF
exit 0
