#!/bin/sh
# After the files go in: the folder for the library. Nothing is started here: pkg ends whatever its
# scripts leave running, and starting services is left to the admin, as with FreeBSD's own packages.
data=$(sysrc -qn wittymediaserver_data)
[ -n "$data" ] || data=/var/db/wittymediaserver
user=$(sysrc -qn wittymediaserver_user)
[ -n "$user" ] || user=wms
install -d -o "$user" -m 0750 "$data"
pw groupmod video -m "$user" >/dev/null 2>&1 || true
exit 0
