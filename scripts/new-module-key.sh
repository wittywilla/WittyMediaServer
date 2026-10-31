#!/usr/bin/env bash
# Makes the key that signs modules' releases: a second key, separate from the one that
# signs the server (docs/MODULES-CUTOVER.md says why). The owner runs this, once.
#
#   scripts/new-module-key.sh ~/.wms-signing/module.key
#
# It writes the new key where you say, refuses to replace any file already there, and
# never opens an existing key. It prints only the public half, which isn't secret.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

path=${1:-}
[ $# -eq 1 ] && [ -n "$path" ] || { echo "usage: scripts/new-module-key.sh <where to keep the new key>" >&2; exit 2; }
[ ! -e "$path" ] || { echo "$path already exists, and this never replaces a key. Choose another path." >&2; exit 1; }

# Inside this checkout it could end up committed.
case "$(cd "$(dirname "$path")" 2>/dev/null && pwd -P)/" in
  "$(pwd -P)"/*) echo "$path is inside this repository. Keep the key outside it." >&2; exit 1 ;;
esac

# wms-sign says the public half goes elsewhere, because it is written for the server's
# key, so what it prints is not passed on. -key means no other key is even looked for.
unset WMS_SIGNING_KEY
go run ./cmd/wms-sign keygen -key "$path" >/dev/null
public=$(go run ./cmd/wms-sign pubkey -key "$path")

cat <<EOF

The new key is in $path.
Back it up now: a password manager, or a USB stick kept safe (docs/MODULES-CUTOVER.md).

Its public half, which isn't secret:

  $public

It goes in internal/update/keys.go, as moduleKey:

  const moduleKey = "$public"

Next: scripts/set-module-secret.sh $path
EOF
