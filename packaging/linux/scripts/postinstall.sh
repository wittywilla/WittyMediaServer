#!/bin/sh
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

WittyMediaServer is installed. Open http://$(hostname 2>/dev/null || echo localhost):9797/ to finish setup.
Status:  systemctl status wittymediaserver
Remove:  sudo wittymediaserver --uninstall (asks about your data)

EOF
exit 0
