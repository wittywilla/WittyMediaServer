#!/usr/bin/env bash
# Publishes every repository in the order they depend on each other: pushes, tags, waits for each
# release, and checks what came out. This is the one command for a release of the whole project.
#
#   scripts/publish-all.sh                 a dry run: does all of it in throwaway clones, pushing to
#                                          throwaway copies. It reads GitHub, and changes nothing.
#   scripts/publish-all.sh --apply         the real thing. Stops at the first failure, and can be run
#                                          again: what's already published is left alone.
#
#   --repos <folder>      where the repositories are           (default: beside this one)
#   --branch <name>       publish this branch of a module repository, where it has one, as its main
#   --server-branch <n>   the server's branch                  (default: release/0.6.0-alpha)
#   --work <folder>       where to work                        (default: a new folder in your home)
#   --from <Repository>   start here, having published everything before it
#   --skip-tests          for a second dry run of something that passed
#   --no-server-tag       --apply stops short of tagging the server, which starts its release
#
# Needs git, go, node, python3, and for anything on GitHub, gh signed in. Run it in WSL or on Linux.
# docs/MODULES-CUTOVER.md says what a person has to do first.
set -euo pipefail

OWNER=wittywilla
LIBRARIES=(WittyModule WittyDisc)
PROGRAMS=(WittyDVD WittyBluray WittyCD WittyTuner WittyPhotos WittyIntegratedFileBrowser WittyRigs WittyPlay WittyCouch WittyCouchHost)
WEB=(WittyInk WittyEbook WittyPDF WittyComics WittyVisualizer WittyStats)
APPS=(WittyReader WittyMediaPlayer WittyMediaClient)
CATALOG=WittyModuleCatalog
SERVER=WittyMediaServer
ORDER=("${LIBRARIES[@]}" "${PROGRAMS[@]}" "${WEB[@]}" "${APPS[@]}" "$CATALOG" "$SERVER")
# The disc modules' copies for WittyMediaPlayer: a Go module of its own in a folder of each repository.
PLAYER_COPIES=(WittyDVD:wittydvdplayer WittyBluray:wittyblurayplayer)
# One word or pattern a line, of what no repository may name. It lives outside them all, as naming them
# here would be just what it looks for.
NAMES_FILE=${WMS_FORBIDDEN_NAMES:-}

apply=0 branch="" server_branch=release/0.6.0-alpha work="" from="" skip_tests=0 server_tag=1
repos=$(cd "$(dirname "$0")/../.." 2>/dev/null && pwd)
while [ $# -gt 0 ]; do
  case "$1" in
    --apply) apply=1 ;;
    --repos) repos=$2; shift ;;
    --branch) branch=$2; shift ;;
    --server-branch) server_branch=$2; shift ;;
    --work) work=$2; shift ;;
    --from) from=$2; shift ;;
    --skip-tests) skip_tests=1 ;;
    --no-server-tag) server_tag=0 ;;
    *) sed -n '2,19p' "$0"; exit 2 ;;
  esac
  shift
done

say() { printf '\n==> %s\n' "$*"; }
note() { printf '    %s\n' "$*"; }
die() { printf '\nSTOPPED: %s\n' "$*" >&2; exit 1; }
left=()
owner_step() { left+=("$1"); note "owner step left: $1"; }

for tool in git go node python3; do command -v "$tool" >/dev/null || die "$tool isn't installed"; done
if [ $apply = 1 ]; then
  command -v gh >/dev/null || die "--apply needs gh"
  gh auth status >/dev/null 2>&1 || die "--apply needs gh signed in: gh auth login"
fi
have_gh=0
if command -v gh >/dev/null && gh auth status >/dev/null 2>&1; then have_gh=1; fi

work=${work:-$(mktemp -d "$HOME/publish-all.XXXXXX")}
mkdir -p "$work"
work=$(cd "$work" && pwd)
clones=$work/clones
remotes=$work/remotes
mkdir -p "$clones" "$remotes"
echo "Working in $work"
[ $apply = 1 ] && echo "This publishes for real." || echo "A dry run: nothing leaves this machine."

# Everyone fetches modules from where they were just pushed, not from a proxy that may lag.
export GOWORK=off GOPRIVATE="github.com/$OWNER/*" GOFLAGS="-modcacherw -buildvcs=false"
export GIT_ALLOW_PROTOCOL=file:https:ssh GIT_CONFIG_NOSYSTEM=1
gitname=$(git -C "$repos/$SERVER" config user.name || echo "WittyWilla")
gitmail=$(git -C "$repos/$SERVER" config user.email || echo "noreply@users.noreply.github.com")
export GIT_AUTHOR_NAME="$gitname" GIT_AUTHOR_EMAIL="$gitmail" GIT_COMMITTER_NAME="$gitname" GIT_COMMITTER_EMAIL="$gitmail"
if [ $apply = 0 ]; then
  # A dry run keeps its own module cache: what it pretends to publish isn't what's on GitHub.
  export GOMODCACHE=$work/modcache
  i=0
  for r in "${ORDER[@]}"; do
    export "GIT_CONFIG_KEY_$i=url.file://$remotes/$r.git.insteadOf" "GIT_CONFIG_VALUE_$i=https://github.com/$OWNER/$(tr 'A-Z' 'a-z' <<<"$r")"
    i=$((i + 1))
  done
  export GIT_CONFIG_COUNT=$i
fi

field() { sed -n "s/.*\"$1\": *\"\([^\"]*\)\".*/\1/p" "$2" | head -1 | tr -d '\r'; }
url() { [ $apply = 1 ] && echo "https://github.com/$OWNER/$1.git" || echo "$remotes/$1.git"; }

source_branch() { # <repo>
  if [ "$1" = "$SERVER" ]; then echo "$server_branch"
  elif [ -n "$branch" ] && git -C "$repos/$1" rev-parse -q --verify "refs/heads/$branch" >/dev/null; then echo "$branch"
  else echo main; fi
}

# ---- what has to be true first, read-only
say "Checking what has to be in place"
NAMES_FILE=${NAMES_FILE:-$repos/forbidden-names.txt}
[ -f "$NAMES_FILE" ] || note "no $NAMES_FILE, so repositories weren't searched for names they mustn't hold"
for r in "${ORDER[@]}"; do
  [ -d "$repos/$r/.git" ] || die "$repos/$r isn't a repository"
done
if [ $have_gh = 1 ]; then
  for r in "${ORDER[@]}"; do
    gh repo view "$OWNER/$r" >/dev/null 2>&1 && continue
    if [ $apply = 1 ]; then
      note "creating $r"
      gh repo create "$OWNER/$r" --public --disable-wiki=false
    else note "would create $r, public and empty"; fi
  done
  pub=$(gh api "repos/$OWNER/$CATALOG/actions/variables/MODULE_PUBLIC_KEY" --jq .value 2>/dev/null || true)
  [ -n "$pub" ] || { [ $apply = 1 ] && die "$CATALOG has no MODULE_PUBLIC_KEY variable"; owner_step "set MODULE_PUBLIC_KEY on $CATALOG, and the WMS_SIGNING_KEY secret in each repository's release environment: scripts/set-module-secret.sh"; }
  for r in "${PROGRAMS[@]}" "${WEB[@]}" "$CATALOG"; do
    if ! gh secret list -R "$OWNER/$r" --env release --json name --jq '.[].name' 2>/dev/null | grep -q '^WMS_SIGNING_KEY$'; then
      [ $apply = 1 ] && die "$r has no WMS_SIGNING_KEY in its release environment"
      owner_step "$r needs WMS_SIGNING_KEY in a release environment"
    fi
  done
else
  [ $apply = 1 ] || note "gh isn't signed in here, so what's on GitHub wasn't looked at"
fi
# From the branch being published: the folder itself may be on another one.
if git -C "$repos/$SERVER" show "$server_branch:internal/update/keys.go" | grep -q 'const moduleKey = ""'; then
  [ $apply = 1 ] && die "moduleKey in internal/update/keys.go is empty: no server could accept any of these"
  owner_step "scripts/new-module-key.sh, and put its public half in internal/update/keys.go"
fi

# ---- one repository
started=0
[ -z "$from" ] && started=1
tag_of=()
version_of() { # <clone>
  if [ -f "$1/module.json" ]; then field version "$1/module.json"
  elif [ -f "$1/package.json" ]; then field version "$1/package.json"
  fi
}

prepare() { # <repo>: a clone of what's to be published, in $dir
  local r=$1 src
  src=$(source_branch "$r")
  dir=$clones/$r
  rm -rf "$dir"
  git clone -q --branch "$src" "$repos/$r" "$dir" 2>/dev/null || die "$r has no branch $src"
  if [ $apply = 0 ] && [ ! -d "$remotes/$r.git" ]; then
    git clone -q --bare "$repos/$r" "$remotes/$r.git"
    # What a repository's tags were, as GitHub has them, and no others.
    git -C "$remotes/$r.git" tag -l | xargs -r git -C "$remotes/$r.git" tag -d >/dev/null
  fi
  git -C "$dir" remote set-url origin "$(url "$r")"
  if [ $apply = 1 ]; then
    git -C "$dir" fetch -q origin --tags 2>/dev/null || true
  else
    # A rehearsal pushes to its own copies, so only this can find a push GitHub would refuse.
    local on_github
    on_github=$(git ls-remote "https://github.com/$OWNER/$r" refs/heads/main refs/heads/master 2>/dev/null | cut -f1)
    for sha in $on_github; do
      git -C "$dir" merge-base --is-ancestor "$sha" HEAD 2>/dev/null || die "$r: GitHub has ${sha:0:7}, which isn't in $src, so the push would be refused"
    done
  fi
  if [ -f "$NAMES_FILE" ] && git -C "$dir" grep -nE -f "$NAMES_FILE" -- . ':!*.lock' ':!package-lock.json' ':!go.sum' >"$work/forbidden.txt" 2>/dev/null; then
    head -5 "$work/forbidden.txt"
    die "$r names something it mustn't"
  fi
  [ -f "$dir/go.mod" ] && if grep -q '^replace' "$dir/go.mod"; then die "$r's go.mod has replace directives"; fi
  if [ -f "$dir/module.json" ] && ! grep -q '"delivery": *"library"' "$dir/module.json"; then
    api=$(sed -n 's/.*"api": *\([0-9]*\).*/\1/p' "$dir/module.json" | head -1)
    [ "${api:-0}" -ge 3 ] || die "$r's branch $src is at API ${api:-none}: it hasn't had the module work merged"
  fi
}

# Where a repository's Go is: WittyMediaPlayer's is in a folder, the others' at the top.
go_folder() { [ "$1" = WittyMediaPlayer ] && echo "$dir/engine" || echo "$dir"; }

pin() { # <repo>: uses what was just published, and commits that
  local r=$1 go_dir
  go_dir=$(go_folder "$r")
  [ -f "$go_dir/go.mod" ] || return 0
  sed -i -E '/github\.com\/wittywilla\/witty(module|disc) /d' "$go_dir/go.sum" 2>/dev/null || true
  (
    cd "$go_dir"
    if [ "$r" = WittyMediaPlayer ]; then
      go get "github.com/$OWNER/wittydvd@v$dvd_version" "github.com/$OWNER/wittybluray@v$bluray_version" "github.com/$OWNER/wittycd@v$cd_version" \
        "github.com/$OWNER/wittydvd/wittydvdplayer@v$dvd_copy_version" "github.com/$OWNER/wittybluray/wittyblurayplayer@v$bluray_copy_version"
    fi
    if [ "$r" != WittyModule ] && grep -rlq "github.com/$OWNER/wittymodule" . --include=*.go; then
      go get "github.com/$OWNER/wittymodule@v$module_version"
    fi
    if [ "$r" != WittyDisc ] && grep -rlq "github.com/$OWNER/wittydisc" . --include=*.go; then
      go get "github.com/$OWNER/wittydisc@v$disc_version"
    fi
    go mod tidy
  )
  if [ -n "$(git -C "$go_dir" status --porcelain go.mod go.sum 2>/dev/null)" ]; then
    git -C "$go_dir" add go.mod go.sum
    git -C "$go_dir" commit -qm "Uses the versions this release publishes"
    note "pinned"
  fi
}

check_alone() { # <repo>: its own tests, on its own
  local r=$1 log=$work/$r.log go_dir
  [ $skip_tests = 1 ] && return 0
  go_dir=$(go_folder "$r")
  if [ -f "$go_dir/go.mod" ]; then
    (cd "$go_dir" && go vet ./... && go test ./... -count=1 -timeout 15m) >"$log" 2>&1 || { tail -30 "$log" | cut -c1-200; die "$r's tests failed: $log"; }
  fi
  if [ -f "$dir/package.json" ] && grep -q '"test"' "$dir/package.json"; then
    (cd "$dir" && npm install --ignore-scripts --no-audit --no-fund --no-package-lock && npm test && rm -rf node_modules) >"$log.npm" 2>&1 || { tail -30 "$log.npm" | cut -c1-200; die "$r's Node tests failed: $log.npm"; }
  fi
  note "its own tests pass"
}

lock() { # <repo>: locks what a module's bundle is built with, where nothing else has
  local r=$1
  [ -f "$dir/package.json" ] && [ ! -f "$dir/package-lock.json" ] && grep -q 'build:bundle' "$dir/package.json" || return 0
  if [ $apply = 0 ]; then note "would lock its npm packages"; return 0; fi
  sed -i -E "s|\"github:$OWNER/WittyInk\"|\"github:$OWNER/WittyInk#v$ink_version\"|" "$dir/package.json"
  if (cd "$dir" && npm install --package-lock-only --ignore-scripts --no-audit --no-fund >/dev/null 2>&1); then
    git -C "$dir" add package.json package-lock.json
    git -C "$dir" commit -qm "Locks what the bundle is built with"
    note "locked"
  else
    note "couldn't lock its npm packages: its workflow installs them unlocked"
  fi
}

push_and_tag() { # <repo> <version or empty>
  local r=$1 v=${2:-} target
  target=main
  [ "$r" = "$SERVER" ] && target=$server_branch
  # On a second run this clone is pinned afresh, so its commit differs even when what's in it doesn't.
  if git -C "$dir" fetch -q origin "refs/heads/$target" 2>/dev/null && git -C "$dir" diff --quiet FETCH_HEAD HEAD; then
    git -C "$dir" reset -q --hard FETCH_HEAD
    note "$target is already this"
  else
    git -C "$dir" push -q origin "HEAD:refs/heads/$target" || die "$r: the push was refused"
    note "pushed to $target"
  fi
  [ -n "$v" ] || return 0
  tag_once "$r" "v$v"
}

tag_once() { # <repo> <tag>
  local r=$1 tag=$2 have want
  have=$(git -C "$dir" ls-remote --tags origin "refs/tags/$tag" | cut -f1)
  if [ -n "$have" ]; then
    note "$tag is already there, and tags never move"
    return 0
  fi
  want=$(git -C "$dir" rev-parse HEAD)
  git -C "$dir" tag -a "$tag" -m "$r ${tag##*v}" "$want"
  git -C "$dir" push -q origin "refs/tags/$tag" || die "$r: the tag was refused"
  note "tagged $tag"
}

# Go finds a module in a folder by a tag with the folder in front, so WittyMediaPlayer needs these.
# Its go.mod says which version of the module around it, so this comes after that's published.
player_copy() { # <repo> <folder>: with $dir still the repository's clone
  local r=$1 folder=$2 v log=$work/$r-$2.log
  v=$(field version "$dir/$folder/module.json")
  (
    cd "$dir/$folder"
    go get "github.com/$OWNER/$(tr 'A-Z' 'a-z' <<<"$r")@v$(version_of "$dir")" "github.com/$OWNER/wittydisc@v$disc_version" "github.com/$OWNER/wittymodule@v$module_version"
    go mod tidy
  )
  if [ -n "$(git -C "$dir/$folder" status --porcelain . 2>/dev/null)" ]; then
    git -C "$dir" add "$folder"
    git -C "$dir" commit -qm "$folder: uses the versions this release publishes"
  fi
  if [ $skip_tests = 0 ]; then
    (cd "$dir/$folder" && go vet ./... && go test ./... -count=1 -timeout 15m) >"$log" 2>&1 || { tail -30 "$log" | cut -c1-200; die "$folder's tests failed: $log"; }
    note "$folder's tests pass"
  fi
  push_and_tag "$r" ""
  tag_once "$r" "$folder/v$v"
}

wait_release() { # <repo> <version> <signed?>: until the release workflow has done its job
  local r=$1 v=$2 signed=$3 tag=v$2 waited=0
  [ $apply = 1 ] || return 0
  note "waiting for $r $tag's release. Approve it in the Actions tab if your rules ask for that."
  until assets=$(gh release view "$tag" -R "$OWNER/$r" --json assets --jq '.assets[].name' 2>/dev/null) && [ -n "$assets" ]; do
    sleep 30
    waited=$((waited + 30))
    [ $waited -lt 3600 ] || die "$r $tag's release didn't appear in an hour: see its Actions tab, then run this again with --from $r"
  done
  if [ "$signed" = 1 ]; then
    grep -q 'SHA256SUMS.txt.sig$' <<<"$assets" || die "$r $tag's release has no signature"
  fi
  note "released, with $(wc -l <<<"$assets") files"
}

publish() { # <repo> <signed?> <tag?>
  local r=$1 signed=$2 tagged=$3 v
  say "$r"
  prepare "$r"
  pin "$r"
  lock "$r"
  check_alone "$r"
  v=""
  [ "$tagged" = 1 ] && v=$(version_of "$dir")
  push_and_tag "$r" "$v"
  [ -n "$v" ] && wait_release "$r" "$v" "$signed"
  return 0
}

should_run() { # <repo>
  [ "$started" = 1 ] && return 0
  [ "$1" = "$from" ] && started=1 && return 0
  return 1
}

# Versions everything else pins to, from what's about to be published.
for pair in WittyModule:module WittyDisc:disc WittyInk:ink WittyDVD:dvd WittyBluray:bluray WittyCD:cd; do
  r=${pair%:*}
  src=$(source_branch "$r")
  v=$(git -C "$repos/$r" show "$src:module.json" | sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' | head -1 | tr -d '\r')
  [ -n "$v" ] || die "no version for $r"
  eval "${pair#*:}_version=$v"
done
dvd_copy_version=$(git -C "$repos/WittyDVD" show "$(source_branch WittyDVD):wittydvdplayer/module.json" | sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' | head -1 | tr -d '\r')
bluray_copy_version=$(git -C "$repos/WittyBluray" show "$(source_branch WittyBluray):wittyblurayplayer/module.json" | sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' | head -1 | tr -d '\r')

for r in "${LIBRARIES[@]}"; do should_run "$r" && publish "$r" 0 1; done
for r in "${PROGRAMS[@]}"; do
  should_run "$r" || continue
  publish "$r" 1 1
  for pair in "${PLAYER_COPIES[@]}"; do
    [ "${pair%:*}" = "$r" ] && player_copy "$r" "${pair#*:}"
  done
done
for r in "${WEB[@]}"; do should_run "$r" && publish "$r" 1 1; done
for r in "${APPS[@]}"; do
  # The apps go on their own schedules. They're pushed, and tagged when they next have a release.
  should_run "$r" && publish "$r" 0 0
done

# ---- the catalogue
if should_run "$CATALOG"; then
  say "$CATALOG"
  prepare "$CATALOG"
  push_and_tag "$CATALOG" ""
  if [ $apply = 1 ]; then
    gh workflow run refresh.yml -R "$OWNER/$CATALOG" --ref main
    note "the refresh is running. It adds every release above, and publishes a signed catalogue."
    catalog_url="https://github.com/$OWNER/$CATALOG/releases/download/catalog/catalog.json"
    pub=$(gh api "repos/$OWNER/$CATALOG/actions/variables/MODULE_PUBLIC_KEY" --jq .value)
    want=$(python3 -c "import json;print(len(json.load(open('$dir/catalog.json'))['modules']))")
    waited=0
    while :; do
      mkdir -p "$work/catalog"
      if curl -fsSL "$catalog_url" -o "$work/catalog/catalog.json" && curl -fsSL "$catalog_url.sig" -o "$work/catalog/catalog.json.sig" &&
        (cd "$clones/WittyModule" && go run ./cmd/wittymodule catalog-check -pub "$pub" "$work/catalog/catalog.json") &&
        [ "$(python3 -c "import json;print(sum(1 for m in json.load(open('$work/catalog/catalog.json'))['modules'] if m['versions']))")" -ge "$((want - 3))" ]; then
        note "the published catalogue has a good signature and lists the new versions"
        break
      fi
      sleep 60
      waited=$((waited + 60))
      [ $waited -lt 3600 ] || die "the catalogue wasn't published in an hour: see $CATALOG's Actions tab, then run this again with --from $CATALOG"
    done
  else
    # What the refresh would do, with a throwaway key in place of the owner's.
    say "A catalogue from what was just pushed, with a throwaway key"
    rm -rf "$work/catalog"
    (cd "$clones/$SERVER" 2>/dev/null) || git clone -q --branch "$server_branch" "$repos/$SERVER" "$clones/$SERVER"
    (cd "$clones/$SERVER/web" && npm ci --ignore-scripts --no-audit --no-fund >/dev/null 2>&1)
    WITTYMODULE_DIR="$clones/WittyModule" bash "$clones/$SERVER/scripts/make-test-catalog.sh" "$work/catalog" "$clones" 2>&1 | grep -E 'listed in|^ok |skipping|rror' | cut -c1-120
  fi
fi

# ---- the server
if should_run "$SERVER"; then
  say "$SERVER"
  prepare "$SERVER"
  pin "$SERVER"
  mkdir -p "$dir/web/dist" && touch "$dir/web/dist/.gitkeep"
  found=$(cd "$dir" && for goos in linux windows darwin freebsd; do GOOS=$goos go list -deps -f '{{with .Module}}{{.Path}}{{end}}' ./... 2>/dev/null; done | sort -u | grep "^github.com/$OWNER/" | grep -vE '/(wittymodule|wittymediaserver)$' || true)
  [ -z "$found" ] || die "the server still has code from: $found"
  note "built from nothing but WittyModule"
  if [ $skip_tests = 0 ]; then
    log=$work/$SERVER.log
    # The server embeds the built web app, so that comes first.
    (cd "$dir/web" && npm ci --ignore-scripts --no-audit --no-fund && npm run build) >"$log" 2>&1 || { tail -30 "$log" | cut -c1-200; die "the web app didn't build: $log"; }
    note "the web app builds"
    (cd "$dir" && go build ./... && go vet ./...) >>"$log" 2>&1 || { tail -30 "$log" | cut -c1-200; die "the server didn't build: $log"; }
    (cd "$dir" && go test ./... -count=1 -timeout 30m) >>"$log" 2>&1 || { grep -vE '^ok|no test files' "$log" | tail -30 | cut -c1-200; die "the server's tests failed with no modules: $log"; }
    note "builds, and its tests pass with no modules"
    if [ $apply = 0 ]; then
      (cd "$dir" && WMS_TEST_CATALOG="$work/catalog" go test ./... -count=1 -timeout 30m) >>"$log" 2>&1 || { grep -vE '^ok|no test files' "$log" | tail -30 | cut -c1-200; die "the server's tests failed with modules: $log"; }
      note "and with real modules from the catalogue"
    fi
  fi
  v=""
  [ $server_tag = 1 ] && v=$(sed -n 's/^var version = "\(.*\)"/\1/p' "$dir/cmd/wittymediaserver/main.go" | tr -d '\r')
  push_and_tag "$SERVER" "$v"
  [ -n "$v" ] && wait_release "$SERVER" "$v" 1
fi

say "Done"
[ $apply = 1 ] && { echo "Everything's published. Bring your own copies up to date: docs/MODULES-CUTOVER.md, step 6."; } || echo "The dry run passed: nothing left this machine. Run it again with --apply when the steps below are done."
if [ ${#left[@]} -gt 0 ]; then
  echo
  echo "Still needs a person first:"
  for l in "${left[@]}"; do echo "  - $l"; done
fi
