#!/usr/bin/env bash
# Publishes plugins and apps to their own repositories (docs/MODULES.md).
# Each folder's history is split out with `git subtree split` and pushed to
# the main branch of its repository, so the mirrors only ever move forward.
#
#   scripts/mirror.sh            mirror everything
#   scripts/mirror.sh WittyPDF   mirror one
#   DRY_RUN=1 scripts/mirror.sh  split only, push nothing
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

OWNER=${OWNER:-wittywilla}
MIRRORS=(
  "web/src/modules/wittyink    WittyInk"
  "web/src/modules/wittyebook  WittyEbook"
  "web/src/modules/wittypdf    WittyPDF"
  "web/src/modules/wittycomics WittyComics"
  "apps/wittyreader            WittyReader"
)

for m in "${MIRRORS[@]}"; do
  read -r dir repo <<<"$m"
  [ $# -gt 0 ] && [ "$1" != "$repo" ] && continue
  echo "== $repo ($dir)"
  sha=$(git subtree split --prefix="$dir" HEAD)
  echo "   split: $sha ($(git ls-tree --name-only "$sha" | tr '\n' ' '))"
  if [ -z "${DRY_RUN:-}" ]; then
    git push "https://github.com/$OWNER/$repo.git" "$sha:refs/heads/main"
  fi
done
