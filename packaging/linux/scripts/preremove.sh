#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Before the files go. An upgrade restarts the service afterwards, so it's left alone then.
# deb: $1 is "remove" or "upgrade". rpm: $1 is 0 on removal, 1 on upgrade.
set -e
case "$1" in
  remove|0)
    if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
      systemctl disable --now wittymediaserver >/dev/null 2>&1 || true
    fi
    # The app with the tray icon, for anyone signed in: its file is about to go.
    if command -v pkill >/dev/null 2>&1; then pkill -f '^/usr/lib/wittymediaserver/' >/dev/null 2>&1 || true; fi
    ;;
esac
exit 0
