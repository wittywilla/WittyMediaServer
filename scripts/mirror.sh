#!/usr/bin/env bash
# Publishes plugins and apps to their own repositories (docs/MODULES.md).
# Each folder's history is split out with `git subtree split` and pushed to
# the main branch of its repository, so the mirrors only ever move forward.
# A folder's wiki/ subfolder is published the same way to the repository's
# GitHub wiki (create the wiki's first page on github.com once to enable it).
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

publish() { # <prefix> <remote url> <branch>
  local sha
  sha=$(git subtree split --prefix="$1" HEAD 2>/dev/null)
  echo "   $1 -> $2 ($3): $sha"
  if [ -z "${DRY_RUN:-}" ]; then git push "$2" "$sha:refs/heads/$3"; fi
}

for m in "${MIRRORS[@]}"; do
  read -r dir repo <<<"$m"
  [ $# -gt 0 ] && [ "$1" != "$repo" ] && continue
  echo "== $repo"
  publish "$dir" "https://github.com/$OWNER/$repo.git" main
  if [ -d "$dir/wiki" ]; then
    wiki="https://github.com/$OWNER/$repo.wiki.git"
    if GIT_TERMINAL_PROMPT=0 git ls-remote "$wiki" >/dev/null 2>&1; then
      publish "$dir/wiki" "$wiki" master
    else
      echo "   wiki skipped: create its first page at https://github.com/$OWNER/$repo/wiki once, then run this again"
    fi
  fi
done
