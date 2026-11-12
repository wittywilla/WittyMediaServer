#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
#
# Draws this repository's icons, backgrounds and in-app brand marks with WittyMediaServer's
# genicon, the one generator for the whole family (WittyMediaServer's docs/BRAND.md). Never edit
# the pictures by hand: change the drawings there and run this.
#
#   scripts/sync-brand.sh            write them
#   scripts/sync-brand.sh --check    change nothing; fail if any is out of date
#
# WITTY_SERVER is the WittyMediaServer checkout (default: beside this repository).
# WITTY_BRAND_CONCEPT picks another design than the one in use (genicon -help lists them).
set -eu
here=$(cd "$(dirname "$0")/.." && pwd)
server=${WITTY_SERVER:-$here}
check=""
[ "${1:-}" = "--check" ] && check="-check"
if [ ! -f "$server/scripts/genicon/main.go" ]; then
  echo "WittyMediaServer isn't at $server: set WITTY_SERVER to its checkout." >&2
  exit 1
fi
cd "$server"
go run ./scripts/genicon -target server -o "$here" ${WITTY_BRAND_CONCEPT:+-concept "$WITTY_BRAND_CONCEPT"} $check
