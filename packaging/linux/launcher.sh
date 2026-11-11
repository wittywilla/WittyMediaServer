#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# What an AppImage and a Flatpak run: the AppImage's AppRun, and the Flatpak's command.
#
#   with nothing after it, on a desktop: the app with the tray icon, which starts the server
#   with nothing after it, with no desktop: the server (wittymediaserver serve)
#   with anything after it: the server's command line (version, config, reset-password, ...)
#
# Nothing is written beside the program: both are read-only. The library, accounts and settings
# are where the server keeps them for an account, $XDG_DATA_HOME/wittymediaserver (in a Flatpak
# that is ~/.var/app/<app id>/data/wittymediaserver), or wherever WMS_DATA_DIR says.
HERE="${APPDIR:-$(dirname "$(readlink -f "$0")")}"
SERVER="$HERE/bin/wittymediaserver"

if [ $# -eq 0 ]; then
  # A Flatpak is only ever opened from a desktop, and hides DISPLAY from a program with no window.
  if [ -x "$HERE/WittyMediaServer" ] && [ -n "${FLATPAK_ID:-}${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]; then
    exec "$HERE/WittyMediaServer"
  fi
  set -- serve
fi

if [ "$1" != serve ]; then
  exec "$SERVER" "$@"
fi

# The server asks to be started again by ending with 75 (update.ExitRestart), after a reset or a
# changed port. It can't start its own next copy here: when this one ends, the AppImage is
# unmounted and the Flatpak's sandbox closes, under the copy it started.
WMS_SUPERVISED=1
export WMS_SUPERVISED
while :; do
  "$SERVER" "$@"
  code=$?
  [ "$code" -eq 75 ] || exit "$code"
done
