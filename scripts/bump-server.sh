#!/usr/bin/env bash
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
old=$(sed -n 's/^var version = "\(.*\)"$/\1/p' cmd/wms/main.go | tr -d '\r')
[ -n "$old" ] || { echo "cmd/wms/main.go no longer says var version = \"...\"" >&2; exit 1; }
[ "$old" != "$ver" ] || { echo "the server is already $ver" >&2; exit 1; }

sed -i "s/^var version = \"$old\"/var version = \"$ver\"/" cmd/wms/main.go
sed -i "0,/\"version\": \"$old\"/s//\"version\": \"$ver\"/" web/package.json
# package-lock.json names it twice: for the package and for its root.
sed -i "0,/\"version\": \"$old\"/s//\"version\": \"$ver\"/" web/package-lock.json
sed -i "0,/\"version\": \"$old\"/s//\"version\": \"$ver\"/" web/package-lock.json
echo "server: $old to $ver"

if [ -n "$note" ]; then
  awk -v line="- **Server $ver:** $note" 'done != 1 && /^## Unreleased/ { print; print ""; print line; done = 1; next } { print }' CHANGELOG.md > CHANGELOG.md.new
  mv CHANGELOG.md.new CHANGELOG.md
fi

# A server past a module's maxServer would quietly run its built-in copy. CI fails on it too.
manifests=$(ls -d modules/*/ web/src/modules/*/ 2>/dev/null || true)
if [ -n "$manifests" ]; then
  # shellcheck disable=SC2086
  go run ./modules/wittymodule/cmd/wittymodule check -server "$ver" $manifests || {
    echo "raise maxServer in the modules above, then run this check again" >&2
    exit 1
  }
fi
