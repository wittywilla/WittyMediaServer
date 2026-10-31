#!/usr/bin/env bash
# For developing a server and modules together: clones each module beside this checkout
# and makes this checkout build against the clones. Nothing it does is committed.
#
#   scripts/dev-modules.sh WittyPlay WittyEbook   use local copies of these
#   scripts/dev-modules.sh --off                  back to the released versions
#
# Go uses a local go.work (.gitignore covers it). The web app links the module into
# web/node_modules, for one with a package.json. A release is still made from the
# module's own repository, and the server moves to it in go.mod and web/package.json.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

OWNER=${OWNER:-wittywilla}
[ $# -gt 0 ] || { sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }

if [ "$1" = "--off" ]; then
  rm -f go.work go.work.sum
  (cd web && npm ci --ignore-scripts --no-audit --no-fund --silent)
  echo "back to the released versions; your clones beside this checkout are still there"
  exit 0
fi

for repo in "$@"; do
  [[ "$repo" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "not a repository's name: $repo" >&2; exit 2; }
  clone="../$repo"
  if [ -d "$clone/.git" ]; then
    echo "$repo: using the clone at $clone"
  else
    git clone "https://github.com/$OWNER/$repo.git" "$clone"
  fi
  if [ -f "$clone/go.mod" ]; then
    [ -f go.work ] || go work init .
    go work use "$clone"
    echo "$repo: Go builds use $clone"
  fi
  if [ -f "$clone/package.json" ]; then
    # Only web/'s own dependencies; a module for the Go side alone isn't in there.
    name=$(sed -n 's/.*"name": *"\([^"]*\)".*/\1/p' "$clone/package.json" | head -1 | tr -d '\r')
    if grep -q "\"$name\"" web/package.json; then
      (cd web && npm install --no-save --ignore-scripts --no-audit --no-fund --silent "../$clone")
      echo "$repo: the web app uses $clone"
    fi
  fi
done
echo "Undo with: scripts/dev-modules.sh --off"
