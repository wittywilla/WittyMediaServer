#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Before the files go in: the service's own account, and taking over from a copy that the
# install.sh of 0.5.2-alpha or older put in /opt, keeping its data folder, port and account.
set -e

DATA=/var/lib/wittymediaserver
OLD_UNIT=/etc/systemd/system/wittymediaserver.service
DROP_IN_DIR=/etc/systemd/system/wittymediaserver.service.d
DROP_IN=$DROP_IN_DIR/upgraded-from-install-sh.conf

if [ -f "$OLD_UNIT" ] && grep -Eq '^ExecStart=.*/(wms|wittymediaserver) serve' "$OLD_UNIT"; then
  old_user=$(sed -n 's/^User=//p' "$OLD_UNIT" | head -n 1)
  old_data=$(sed -n 's/^Environment=WMS_DATA_DIR=//p' "$OLD_UNIT" | head -n 1)
  old_listen=$(sed -n 's/^Environment=WMS_LISTEN=//p' "$OLD_UNIT" | head -n 1)
  old_prefix=$(sed -n 's/^ExecStart=\(.*\)\/\(bin\/wittymediaserver\|wms\) serve.*/\1/p' "$OLD_UNIT" | head -n 1)
  if command -v systemctl >/dev/null 2>&1; then
    systemctl stop wittymediaserver >/dev/null 2>&1 || true
    systemctl disable wittymediaserver >/dev/null 2>&1 || true
  fi
  # What was different from the defaults carries over.
  if [ -n "$old_user$old_data$old_listen" ]; then
    mkdir -p "$DROP_IN_DIR"
    {
      echo "# Settings carried over from an earlier install. Change them with: sudo systemctl edit wittymediaserver"
      echo "[Service]"
      [ -n "$old_user" ] && [ "$old_user" != wms ] && echo "User=$old_user"
      if [ -n "$old_data" ] && [ "$old_data" != "$DATA" ]; then
        echo "Environment=WMS_DATA_DIR=$old_data"
        echo "ReadWritePaths=$old_data"
      fi
      # postinstall moves a port on its own into the data folder's settings.ini.
      [ -n "$old_listen" ] && [ "$old_listen" != ":9797" ] && echo "Environment=WMS_LISTEN=$old_listen"
      true
    } > "$DROP_IN"
    if [ "$(grep -c '^[A-Z]' "$DROP_IN")" -le 1 ]; then
      rm -f "$DROP_IN"
      rmdir "$DROP_IN_DIR" 2>/dev/null || true
    fi
  fi
  rm -f "$OLD_UNIT"
  # The old program, known files only: someone's data folder may be in there.
  if [ -n "$old_prefix" ] && [ -d "$old_prefix" ]; then
    for f in wms wittyplay-host WittyMediaServer icon.png wittymediaserver.desktop install.sh package-info.json \
             LICENSE NOTICE README.md CHANGELOG.md THIRD_PARTY_LICENSES.md; do
      rm -f "$old_prefix/$f"
    done
    rm -rf "$old_prefix/bin" "$old_prefix/docs" "$old_prefix/share" "$old_prefix/.update"
    rmdir "$old_prefix" 2>/dev/null || true
  fi
  rm -f /usr/local/share/applications/wittymediaserver.desktop
fi

# An upgrade replaces the settings.ini beside the program. A port someone set in it by hand moves
# into the data folder's copy first, unless that copy has one of its own, which wins anyway.
PROGRAM_INI=/usr/lib/wittymediaserver/settings.ini
SERVICE_DATA=$DATA
for f in /etc/systemd/system/wittymediaserver.service.d/*.conf; do
  [ -f "$f" ] || continue
  d=$(sed -n 's/^Environment="\{0,1\}WMS_DATA_DIR=\([^"]*\)"\{0,1\}[[:space:]]*$/\1/p' "$f" | tail -n 1)
  if [ -n "$d" ]; then SERVICE_DATA="$d"; fi
done
port=$(sed -n 's/^[[:space:]]*port[[:space:]]*=[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$PROGRAM_INI" 2>/dev/null | head -n 1)
if [ -n "$port" ] && [ "$port" != 9797 ] && [ -x /usr/lib/wittymediaserver/bin/wittymediaserver ] &&
   ! grep -Eq '^[[:space:]]*port[[:space:]]*=' "$SERVICE_DATA/settings.ini" 2>/dev/null; then
  /usr/lib/wittymediaserver/bin/wittymediaserver config port -data "$SERVICE_DATA" -in-data -port "$port" || true
fi

if ! id wms >/dev/null 2>&1; then
  useradd --system --home-dir "$DATA" --no-create-home --shell /usr/sbin/nologin --comment "WittyMediaServer" wms 2>/dev/null \
    || useradd -r -d "$DATA" -M -s /sbin/nologin -c "WittyMediaServer" wms
fi
exit 0
