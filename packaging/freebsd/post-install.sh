#!/bin/sh
# After the files go in: the folder for the library. On an upgrade, the running service comes back as
# the new version. A new install waits to be enabled, as FreeBSD's packages do.
data=$(sysrc -qn wittymediaserver_data)
[ -n "$data" ] || data=/var/db/wittymediaserver
user=$(sysrc -qn wittymediaserver_user)
[ -n "$user" ] || user=wms
install -d -o "$user" -m 0750 "$data"
pw groupmod video -m "$user" >/dev/null 2>&1 || true
if [ "$(sysrc -qn wittymediaserver_enable)" = YES ]; then
  service wittymediaserver restart >/dev/null 2>&1 || service wittymediaserver start >/dev/null 2>&1 || true
fi
exit 0
