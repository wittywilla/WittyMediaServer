#!/usr/bin/env bash
# Publishes plugins and apps to their own repositories (docs/MODULES.md).
# Each folder's history is split out with `git subtree split` and pushed to
# the main branch of its repository, so the mirrors only ever move forward.
# A folder's wiki/ subfolder is published the same way to the repository's
# GitHub wiki (create the wiki's first page on github.com once to enable it).
#
#   scripts/mirror.sh                    mirror everything
#   scripts/mirror.sh WittyPDF           mirror one
#   scripts/mirror.sh WittyDisc v0.1.0   mirror one and tag what was published (a release)
#   DRY_RUN=1 scripts/mirror.sh          split and check only, push nothing
#
# A release of a Go module first checks that it builds and passes its tests on
# its own (GOWORK=off: only released dependencies, a complete go.sum), the way
# `go install …@version` will build it. Tags are never moved or deleted.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

OWNER=${OWNER:-wittywilla}
NAME=${GIT_AUTHOR_NAME:-WittyWilla}
EMAIL=${GIT_AUTHOR_EMAIL:-wittywilla@users.noreply.github.com}
# WittyDisc comes before the engines that require it, and WittyModule before the
# plugins that do.
MIRRORS=(
  "web/src/modules/wittyink    WittyInk"
  "web/src/modules/wittyebook  WittyEbook"
  "web/src/modules/wittypdf    WittyPDF"
  "web/src/modules/wittycomics WittyComics"
  "web/src/modules/wittyvisualizer WittyVisualizer"
  "apps/wittyreader            WittyReader"
  "modules/wittymodule         WittyModule"
  "modules/wittydisc           WittyDisc"
  "modules/wittydvd            WittyDVD"
  "modules/wittybluray         WittyBluray"
  "modules/wittycd             WittyCD"
  "modules/wittyintegratedfilebrowser WittyIntegratedFileBrowser"
  "modules/wittyphotos         WittyPhotos"
  "modules/wittytuner          WittyTuner"
  "modules/wittyplay           WittyPlay"
  "modules/wittysunshine       WittySunshine"
  "modules/wittymoonlight      WittyMoonlight"
  "modules/wittymoonlighthost WittyMoonlightHost"
)

TAG=${2:-}
if [ -n "$TAG" ]; then
  [[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "not a version tag: $TAG (want vX.Y.Z)" >&2; exit 1; }
fi

# Publishes a folder's history to a branch without ever rewriting it: if the
# branch already holds commits that aren't in the folder's history (GitHub's
# first README, a wiki's first page, an earlier publish), a merge commit on
# top of them makes the folder's current files the branch's files.
# Leaves the published commit in $published.
publish() { # <prefix> <remote url> <branch>
  local sha tip
  sha=$(git subtree split --prefix="$1" HEAD 2>/dev/null)
  tip=""
  if git fetch -q "$2" "$3" 2>/dev/null; then tip=$(git rev-parse FETCH_HEAD); fi
  if [ -n "$tip" ] && [ "$tip" != "$sha" ] && ! git merge-base --is-ancestor "$tip" "$sha"; then
    sha=$(git -c user.name="$NAME" -c user.email="$EMAIL" \
      commit-tree "$sha^{tree}" -p "$tip" -p "$sha" -m "Publish $(basename "$1") from WittyMediaServer $(git rev-parse --short HEAD)")
  fi
  echo "   $1 -> $2 ($3): $sha"
  if [ -z "${DRY_RUN:-}" ]; then git push "$2" "$sha:refs/heads/$3"; fi
  published=$sha
}

# A release publishes what is committed, so the folder must have no local
# changes, and a Go module must build as a stranger's `go install` would.
check_release() { # <dir>
  if [ -f "$1/module.json" ] && ! grep -q "\"version\": *\"${TAG#v}\"" "$1/module.json"; then
    echo "   $1/module.json has another version than $TAG; update it first" >&2; exit 1
  fi
  if ! git diff --quiet HEAD -- "$1" || [ -n "$(git ls-files --others --exclude-standard -- "$1")" ]; then
    echo "   $1 has uncommitted changes; commit them first" >&2; exit 1
  fi
  [ -f "$1/go.mod" ] || return 0
  if grep -q '^replace' "$1/go.mod"; then
    echo "   $1/go.mod has replace directives, which go install refuses" >&2; exit 1
  fi
  (cd "$1" && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test -count=1 ./...) \
    || { echo "   $1 does not build on its own; release its dependencies and run go mod tidy first" >&2; exit 1; }
}

tag() { # <dir> <remote url> <commit>
  local have obj mod
  have=$(git ls-remote --tags "$2" "refs/tags/$TAG" "refs/tags/$TAG^{}" | awk '/\^\{\}$/{c=$1} !/\^\{\}$/{t=$1} END{print c ? c : t}')
  if [ -n "$have" ]; then
    if [ "$have" = "$3" ]; then echo "   $TAG already tags $3"; return; fi
    echo "   $TAG already exists on $2 at another commit ($have); tags never move, pick a new version" >&2; exit 1
  fi
  obj=$(printf 'object %s\ntype commit\ntag %s\ntagger %s\n\n%s %s, published from WittyMediaServer %s\n' \
    "$3" "$TAG" "$(GIT_COMMITTER_NAME="$NAME" GIT_COMMITTER_EMAIL="$EMAIL" git var GIT_COMMITTER_IDENT)" \
    "$(basename "$2" .git)" "$TAG" "$(git rev-parse --short HEAD)" | git mktag)
  echo "   tag $TAG -> $3"
  [ -n "${DRY_RUN:-}" ] && return
  git push "$2" "$obj:refs/tags/$TAG"
  if [ -f "$1/go.mod" ]; then # ask the Go module proxy for it now, so go install …@latest finds it
    mod=$(awk '/^module /{print $2; exit}' "$1/go.mod")
    GOWORK=off GOPROXY=https://proxy.golang.org go list -m "$mod@$TAG" || echo "   the module proxy has not seen $TAG yet; it will on first use"
  fi
}

found=""
for m in "${MIRRORS[@]}"; do
  read -r dir repo <<<"$m"
  [ $# -gt 0 ] && [ "$1" != "$repo" ] && continue
  found=1
  echo "== $repo"
  url="https://github.com/$OWNER/$repo.git"
  [ -n "$TAG" ] && check_release "$dir"
  publish "$dir" "$url" main
  [ -n "$TAG" ] && tag "$dir" "$url" "$published"
  if [ -d "$dir/wiki" ]; then
    wiki="https://github.com/$OWNER/$repo.wiki.git"
    if GIT_TERMINAL_PROMPT=0 git ls-remote "$wiki" >/dev/null 2>&1; then
      publish "$dir/wiki" "$wiki" master
    else
      echo "   wiki skipped: create its first page at https://github.com/$OWNER/$repo/wiki once, then run this again"
    fi
  fi
done
[ -n "$found" ] || { echo "no mirror named ${1:-}" >&2; exit 1; }
