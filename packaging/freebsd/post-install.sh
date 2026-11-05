#!/bin/sh
# After the files go in: the folder for the library. Nothing is started here: pkg ends whatever its
# scripts leave running, and starting services is left to the admin, as with FreeBSD's own packages.
data=$(sysrc -qn wittymediaserver_data)
[ -n "$data" ] || data=/var/db/wittymediaserver
user=$(sysrc -qn wittymediaserver_user)
[ -n "$user" ] || user=wms
install -d -o "$user" -m 0750 "$data"
# The port is chosen in Settings, Network and settings.ini now. One that rc.conf gave the service
# moves into the data folder's settings.ini, where both can change it. An address with a host in
# front stays in rc.conf.
listen=$(sysrc -qn wittymediaserver_listen)
case "$listen" in
  :[0-9]*|0.0.0.0:[0-9]*)
    if /usr/local/lib/wittymediaserver/bin/wittymediaserver config port -data "$data" -in-data -port "${listen##*:}"; then
      chown -h "$user" "$data/settings.ini" 2>/dev/null || true # -h: a link put there is never followed
      sysrc -x wittymediaserver_listen >/dev/null
    fi ;;
esac
pw groupmod video -m "$user" >/dev/null 2>&1 || true
exit 0
