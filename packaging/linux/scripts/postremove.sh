#!/bin/sh
# After the files have gone. `apt remove` and `dnf remove` keep your library, accounts and
# settings. `apt purge` removes everything the server made, down to its account, as purge
# always does. Only names the server made: recorded TV, library folders and anything
# someone put in the data folder stay, and it says so.
# deb: $1 is remove, purge or upgrade. rpm: $1 is 0 on removal, 1 on upgrade.
set -e

DATA=/var/lib/wittymediaserver
DROP_IN_DIR=/etc/systemd/system/wittymediaserver.service.d
DROP_IN=$DROP_IN_DIR/upgraded-from-install-sh.conf

reload() {
  if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
  fi
}

purge_data() {
  dir=$1
  [ -d "$dir" ] || return 0
  for name in wms.db wms.db-wal wms.db-shm cache transcode logs wms.log crash diagnostics certs branding channel-logos \
              bumpers lyrics-corrections modules themes gaming gamerigs wittycouch reset-request.json reset-result.json; do
    rm -rf "${dir:?}/$name"
  done
  rm -rf "${dir:?}"/before-reset-*
  # An empty folder for recordings is nothing to keep.
  rmdir "$dir/recordings" 2>/dev/null || true
  if ! rmdir "$dir" 2>/dev/null; then
    echo "Left in $dir, because it wasn't made by WittyMediaServer or holds your recordings:"
    ls -A "$dir" | sed 's/^/  /'
  fi
}

case "$1" in
  purge)
    if [ -f "$DROP_IN" ]; then
      custom=$(sed -n 's/^Environment=WMS_DATA_DIR=//p' "$DROP_IN" | head -n 1)
      [ -n "$custom" ] && purge_data "$custom"
      rm -f "$DROP_IN"
    fi
    rmdir "$DROP_IN_DIR" 2>/dev/null || true
    purge_data "$DATA"
    if [ -f /etc/wittymediaserver/wms.json ]; then rm -f /etc/wittymediaserver/wms.json; fi
    rmdir /etc/wittymediaserver 2>/dev/null || true
    if id wms >/dev/null 2>&1; then userdel wms >/dev/null 2>&1 || true; fi
    if getent group wms >/dev/null 2>&1; then groupdel wms >/dev/null 2>&1 || true; fi
    reload
    ;;
  remove|0)
    reload
    echo "Your library, accounts and settings are still in $DATA."
    case "$1" in
      remove) echo "To delete them too: sudo apt purge wittymediaserver" ;;
      0) echo "To delete them too: sudo rm -rf $DATA /etc/wittymediaserver, then sudo userdel wms. Your media files aren't in there." ;;
    esac
    ;;
esac
exit 0
