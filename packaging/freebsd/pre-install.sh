#!/bin/sh
# Before the files go in: the service's account, and taking over from a copy that the install.sh of
# this or an older version installed, keeping its data folder, port and account through rc.conf.
RC=/usr/local/etc/rc.d/wittymediaserver
if [ -f "$RC" ] && ! pkg info -e wittymediaserver >/dev/null 2>&1; then
  service wittymediaserver onestop >/dev/null 2>&1 || true
  old_user=$(sed -n 's/^: ..wittymediaserver_user:=\(.*\)}$/\1/p' "$RC" | head -n 1)
  old_data=$(sed -n 's/^: ..wittymediaserver_data:=\(.*\)}$/\1/p' "$RC" | head -n 1)
  old_listen=$(sed -n 's/^: ..wittymediaserver_listen:=\(.*\)}$/\1/p' "$RC" | head -n 1)
  old_prefix=$(sed -n 's|.* \(/[^ ]*\)/wms serve.*|\1|p; s|.* \(/[^ ]*\)/bin/wittymediaserver serve.*|\1|p' "$RC" | head -n 1)
  # Unless rc.conf already said otherwise, what that script had as its defaults goes there.
  [ -n "$old_user" ] && [ "$old_user" != wms ] && [ -z "$(sysrc -qn wittymediaserver_user)" ] && sysrc wittymediaserver_user="$old_user" >/dev/null
  [ -n "$old_data" ] && [ "$old_data" != /var/db/wittymediaserver ] && [ -z "$(sysrc -qn wittymediaserver_data)" ] && sysrc wittymediaserver_data="$old_data" >/dev/null
  [ -n "$old_listen" ] && [ "$old_listen" != :9797 ] && [ -z "$(sysrc -qn wittymediaserver_listen)" ] && sysrc wittymediaserver_listen="$old_listen" >/dev/null
  rm -f "$RC"
  if [ -n "$old_prefix" ] && [ -d "$old_prefix" ]; then
    # Known files only: someone's data folder may be in there.
    for f in wms wittyplay-host WittyMediaServer icon.png wittymediaserver.desktop install.sh package-info.json \
             LICENSE NOTICE README.md CHANGELOG.md THIRD_PARTY_LICENSES.md; do
      rm -f "$old_prefix/$f"
    done
    rm -rf "$old_prefix/bin" "$old_prefix/docs" "$old_prefix/share" "$old_prefix/.update"
    rmdir "$old_prefix" 2>/dev/null || true
  fi
  rm -f /usr/local/share/applications/wittymediaserver.desktop
fi
if ! pw usershow wms >/dev/null 2>&1; then
  pw useradd wms -d /var/db/wittymediaserver -s /usr/sbin/nologin -c "WittyMediaServer"
fi
exit 0
