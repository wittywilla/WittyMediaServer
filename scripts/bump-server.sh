#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Raises WittyMediaServer's own version everywhere it is written, and says which
# modules' ranges no longer cover it. A bump isn't a release: nothing is tagged.
#
#   scripts/bump-server.sh 0.4.1-alpha ["What changed, for the changelog."]
#
# To raise a module's version, use: wittymodule bump [-note text] [-changelog CHANGELOG.md] [-consumer .] <folder>
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

ver=${1:-}
note=${2:-}
[[ "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "usage: scripts/bump-server.sh <version> [note]" >&2; exit 2; }
old=$(sed -n 's/^var version = "\(.*\)"$/\1/p' cmd/wittymediaserver/main.go | tr -d '\r')
[ -n "$old" ] || { echo "cmd/wittymediaserver/main.go no longer says var version = \"...\"" >&2; exit 1; }
[ "$old" != "$ver" ] || { echo "the server is already $ver" >&2; exit 1; }

# perl, not sed -i: GNU and BSD (macOS) sed disagree on -i, and only GNU has 0,/re/.
# -0 reads the whole file, so a substitution without /g changes only the first match.
export OLD="$old" NEW="$ver"
perl -pi -e 's/^var version = "\Q$ENV{OLD}\E"/var version = "$ENV{NEW}"/' cmd/wittymediaserver/main.go
perl -0pi -e 's/"version": "\Q$ENV{OLD}\E"/"version": "$ENV{NEW}"/' web/package.json
# package-lock.json names it twice: for the package and for its root.
perl -0pi -e 's/"version": "\Q$ENV{OLD}\E"/"version": "$ENV{NEW}"/; s/"version": "\Q$ENV{OLD}\E"/"version": "$ENV{NEW}"/' web/package-lock.json
echo "server: $old to $ver"

if [ -n "$note" ]; then
  awk -v line="- **Server $ver:** $note" 'done != 1 && /^## Unreleased/ { print; print ""; print line; done = 1; next } { print }' CHANGELOG.md > CHANGELOG.md.new
  mv CHANGELOG.md.new CHANGELOG.md
fi

# A server past a module's maxServer has to go without it until a new version of it comes out.
