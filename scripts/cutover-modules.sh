#!/usr/bin/env bash
# Moves every module out of this repository, into its own, and makes the server
# consume released versions (docs/MODULES-CUTOVER.md).
#
#   scripts/cutover-modules.sh                       a dry run: everything, for real, in a
#                                                    throwaway clone, published to local
#                                                    repositories. Nothing here or on GitHub changes.
#   scripts/cutover-modules.sh --apply               do it
#   scripts/cutover-modules.sh --require-merged a,b  refuse unless these branches are in HEAD
#   scripts/cutover-modules.sh --work DIR            where a dry run works (default: a new temp folder)
#   scripts/cutover-modules.sh --keep                keep that folder after a dry run that passed
#
# It refuses to run with uncommitted changes or a required branch not merged, so work
# still going on elsewhere isn't lost. --apply can be run again after a failure: what is
# already released is left alone.
#
# A dry run also needs python3 with PyYAML. --apply needs the GitHub CLI, signed in.
set -euo pipefail

OWNER=${OWNER:-wittywilla}
NAME=${GIT_AUTHOR_NAME:-WittyWilla}
EMAIL=${GIT_AUTHOR_EMAIL:-wittywilla@users.noreply.github.com}

# Leaves first: each comes after everything it requires.
MODULES=(
  "modules/wittymodule WittyModule"
  "modules/wittydisc WittyDisc"
  "modules/wittydvd WittyDVD"
  "modules/wittybluray WittyBluray"
  "modules/wittyintegratedfilebrowser WittyIntegratedFileBrowser"
  "modules/wittyphotos WittyPhotos"
  "modules/wittytuner WittyTuner"
  "modules/wittyplay WittyPlay"
  "web/src/modules/wittyink WittyInk"
  "web/src/modules/wittyebook WittyEbook"
  "web/src/modules/wittypdf WittyPDF"
  "web/src/modules/wittycomics WittyComics"
  "web/src/modules/wittyvisualizer WittyVisualizer"
  "apps/wittyreader WittyReader"
)
PURE_WEB="WittyInk WittyEbook WittyPDF WittyComics WittyReader"

apply=""
required=""
work=""
keep=""
while [ $# -gt 0 ]; do
  case "$1" in
    --apply) apply=1 ;;
    --require-merged) required=${2:?--require-merged needs branch names, separated by commas}; shift ;;
    --work) work=${2:?--work needs a folder}; shift ;;
    --keep) keep=1 ;;
    *) echo "unknown option: $1 (see the top of this script)" >&2; exit 2 ;;
  esac
  shift
done

say() { printf '\n==> %s\n' "$*"; }
run() { printf '   $ %s\n' "$*"; "$@"; }
die() { printf '\nSTOPPED: %s\n' "$*" >&2; exit 1; }

top=$(git rev-parse --show-toplevel)
cd "$top"
kit_dir="$top/scripts/module-kit"

# ---------------------------------------------------------------- before anything

GH=""
if command -v gh >/dev/null 2>&1; then GH=gh
elif [ -x "/c/Program Files/GitHub CLI/gh.exe" ]; then GH="/c/Program Files/GitHub CLI/gh.exe"; fi

say "Checking that it is safe to start"
[ -z "$(git status --porcelain)" ] || { git status --short >&2; die "this checkout has uncommitted changes. Other work may still be going on: finish or commit it first."; }
IFS=',' read -r -a branches <<<"$required"
for b in ${branches[@]+"${branches[@]}"}; do
  git rev-parse --verify -q "$b^{commit}" >/dev/null || die "no branch named $b here, so it can't be checked that it is merged"
  git merge-base --is-ancestor "$b" HEAD || die "$b isn't merged into this branch yet"
  echo "   $b is merged"
done
for t in git go node npm; do command -v "$t" >/dev/null 2>&1 || die "$t is needed and isn't installed here"; done
git subtree -h >/dev/null 2>&1 || [ $? -eq 129 ] || die "git subtree is needed and isn't installed here"
for m in "${MODULES[@]}"; do
  read -r dir _ <<<"$m"
  [ -d "$dir" ] || die "$dir isn't here. Has the cutover already finished on this branch?"
done

repo_exists() {
  if [ -n "$GH" ]; then
    "$GH" repo view "$OWNER/$1" --json name >/dev/null 2>&1
  else
    GIT_TERMINAL_PROMPT=0 git ls-remote "https://github.com/$OWNER/$1.git" >/dev/null 2>&1
  fi
}
say "Checking every repository exists"
if [ -n "$GH" ]; then echo "   asking with the GitHub CLI"; elif [ -n "$apply" ]; then die "--apply needs the GitHub CLI (gh), signed in"; else
  echo "   the GitHub CLI isn't here, so this asks with git ls-remote instead: the same answer for public repositories"
fi
missing=""
for m in "${MODULES[@]}"; do
  read -r _ repo <<<"$m"
  if repo_exists "$repo"; then echo "   $OWNER/$repo exists"; else missing="$missing $repo"; fi
done
if [ -n "$missing" ]; then
  if [ -n "$apply" ]; then die "these repositories are missing:$missing. Creating them is the owner's step: docs/MODULES-CUTOVER.md."; fi
  echo "   MISSING:$missing (a real run would stop here; this dry run carries on with empty local repositories)"
fi

# ---------------------------------------------------------------- where it happens

trap_cleanup() { :; }
if [ -n "$apply" ]; then
  say "This is the real thing: it pushes to GitHub, and tags can't be taken back"
  branch=$(git rev-parse --abbrev-ref HEAD)
  case "$branch" in
    modules-cutover) echo "   carrying on with branch modules-cutover" ;;
    *) run git checkout -b modules-cutover ;;
  esac
  repo_url() { echo "https://github.com/$OWNER/$1.git"; }
else
  say "Dry run: nothing here or on GitHub changes"
  for t in python3 zip sha256sum; do command -v "$t" >/dev/null 2>&1 || die "a dry run needs $t"; done
  python3 -c 'import yaml' 2>/dev/null || die "a dry run needs PyYAML (pip install pyyaml)"
  go_cache=$(go env GOCACHE)
  real_mod=$(go env GOMODCACHE)
  npm_cache=$(npm config get cache)
  source_top=$top
  [ -n "$work" ] || work=$(mktemp -d "${TMPDIR:-/tmp}/wms-cutover-XXXXXX")
  mkdir -p "$work"/{home,bare,state,bin,standalone}
  work=$(cd "$work" && pwd)
  echo "   working in $work"
  trap_cleanup() { if [ $? -ne 0 ]; then echo; echo "The dry run's folder is kept, to look at: $work"; fi; }

  # Real repositories, read once: what a real publish would build on, tags and all.
  for m in "${MODULES[@]}"; do
    read -r _ repo <<<"$m"
    bare="$work/bare/$repo.git"
    [ -d "$bare" ] && continue
    if GIT_TERMINAL_PROMPT=0 git clone -q --bare "https://github.com/$OWNER/$repo.git" "$bare" 2>/dev/null; then
      echo "   copied $OWNER/$repo from GitHub, read only: $(git -C "$bare" tag | tr '\n' ' ')"
    else
      git init -q --bare -b main "$bare"
      echo "   $OWNER/$repo couldn't be read, so it starts empty here"
    fi
    # Go asks for the lower-case name.
    lower=$(printf '%s' "$repo" | tr '[:upper:]' '[:lower:]')
    [ -e "$work/bare/$lower.git" ] || ln -s "$repo.git" "$work/bare/$lower.git"
  done

  # From here on, this run's own settings: "github.com/<owner>" means the folder above,
  # for git, npm and Go, and Go keeps the local versions out of the real module cache.
  export HOME="$work/home"
  cat > "$HOME/.gitconfig" <<EOF
[user]
	name = $NAME
	email = $EMAIL
[init]
	defaultBranch = main
[protocol "file"]
	allow = always
[url "file://$work/bare/"]
	insteadOf = https://github.com/$OWNER/
	insteadOf = ssh://git@github.com/$OWNER/
	insteadOf = git@github.com:$OWNER/
EOF
  export GOCACHE="$go_cache" GOMODCACHE="$work/gomod" GOFLAGS=-modcacherw
  export GOPRIVATE="github.com/$OWNER/*"
  export GOPROXY="file://$real_mod/cache/download,https://proxy.golang.org"
  export npm_config_cache="$npm_cache"
  repo_url() { echo "file://$work/bare/$1.git"; }

  if [ ! -d "$work/wms" ]; then
    say "A throwaway clone of $(git rev-parse --short HEAD)"
    run git clone -q --no-local --config core.autocrlf=false "$source_top" "$work/wms"
    run git -C "$work/wms" checkout -q -b modules-cutover "$(git rev-parse HEAD)"
  fi
  cd "$work/wms"
  top="$work/wms"
  kit_dir="$top/scripts/module-kit"

  # The parts of the GitHub CLI the withdraw workflow uses, on a local folder.
  cat > "$work/bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
rel="$FAKE_GH_DIR"
case "$1 $2" in
  "release view")
    [ -d "$rel/$3" ] || { echo "release not found" >&2; exit 1; }
    if [ "${4:-}" = --json ]; then ls "$rel/$3"; fi ;;
  "release download") cp "$rel/$3/$5" "$7/" ;;
  "release create") mkdir -p "$rel/$3"; printf '%s\n' "$*" > "$rel/$3.created" ;;
  "release upload") tag=$3; shift 3; for f; do [ "$f" = --clobber ] || cp "$f" "$rel/$tag/"; done ;;
  *) echo "the stand-in for gh doesn't know: $*" >&2; exit 1 ;;
esac
EOF
  chmod +x "$work/bin/gh"

  # A check that a signature is good, separate from the tool that made it.
  mkdir -p "$work/verify"
  cat > "$work/verify/verify.go" <<'EOF'
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func main() {
	pub, _ := base64.StdEncoding.DecodeString(os.Args[1])
	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	b, err := os.ReadFile(os.Args[2] + ".sig")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sig, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if !ed25519.Verify(pub, data, sig) {
		fmt.Println("bad signature on", os.Args[2])
		os.Exit(1)
	}
}
EOF
  (cd "$work/verify" && go build -o "$work/bin/verify" verify.go)

  say "A throwaway signing key, made by wms-sign keygen as the owner's will be"
  rm -f "$work/module.key"
  run go run ./cmd/wms-sign keygen -key "$work/module.key" >/dev/null
  public_key=$(go run ./cmd/wms-sign pubkey -key "$work/module.key")
  echo "   its public half: $public_key"
fi
trap trap_cleanup EXIT

# ---------------------------------------------------------------- one module

field() { sed -n "s/.*\"$1\": *\"\([^\"]*\)\".*/\1/p" "$2" | head -1 | tr -d '\r'; }
version_file() { if [ -f "$1/module.json" ]; then echo "$1/module.json"; else echo "$1/package.json"; fi; }
delivery_of() { if [ -f "$1/module.json" ]; then field delivery "$1/module.json"; else echo app; fi; }
signs() { case "$1" in helper | program | web) return 0 ;; *) return 1 ;; esac; }

# Publishes a folder's history to a branch without ever rewriting it: if the branch
# already holds commits that aren't in the folder's history (GitHub's first README, a
# wiki's first page, an earlier publish), a merge commit on top of them makes the
# folder's current files the branch's files. Leaves the published commit in $published.
publish() { # <prefix> <remote url> <branch>
  local sha tip
  echo "   splitting $1's history (this takes a while)"
  sha=$(git subtree split --prefix="$1" HEAD 2>/dev/null)
  tip=""
  if git fetch -q "$2" "$3" 2>/dev/null; then tip=$(git rev-parse FETCH_HEAD); fi
  # Already holding these files, from an earlier run: nothing to add.
  if [ -n "$tip" ] && [ "$(git rev-parse "$tip^{tree}")" = "$(git rev-parse "$sha^{tree}")" ]; then
    echo "   $3 already holds these files"
    published=$tip
    return
  fi
  if [ -n "$tip" ] && [ "$tip" != "$sha" ] && ! git merge-base --is-ancestor "$tip" "$sha"; then
    sha=$(git -c user.name="$NAME" -c user.email="$EMAIL" \
      commit-tree "$sha^{tree}" -p "$tip" -p "$sha" -m "Publish $(basename "$1") from WittyMediaServer $(git rev-parse --short HEAD)")
    echo "   $3 has commits the folder's history doesn't: merged on top of them, nothing rewritten"
  fi
  run git push -q "$2" "$sha:refs/heads/$3"
  published=$sha
}

# Tags never move: the same version at another commit stops the whole thing.
tag_release() { # <remote url> <repository> <commit> <tag>
  local have obj
  have=$(git ls-remote --tags "$1" "refs/tags/$4" "refs/tags/$4^{}" | awk '/\^\{\}$/{c=$1} !/\^\{\}$/{t=$1} END{print c ? c : t}')
  if [ -n "$have" ]; then
    [ "$have" = "$3" ] || die "$4 already exists on $2 at another commit ($have). Tags never move."
    echo "   $4 already tags $3"
    return
  fi
  obj=$(printf 'object %s\ntype commit\ntag %s\ntagger %s\n\n%s %s, published from WittyMediaServer %s\n' \
    "$3" "$4" "$(GIT_COMMITTER_NAME="$NAME" GIT_COMMITTER_EMAIL="$EMAIL" git var GIT_COMMITTER_IDENT)" \
    "$2" "$4" "$(git rev-parse --short HEAD)" | git mktag)
  run git push -q "$1" "$obj:refs/tags/$4"
}

# The newest version tag on a repository, without the v. Pre-releases sort before their release.
newest_tag() { # <remote url>
  git ls-remote --tags --refs "$1" 'v*' 2>/dev/null | sed 's|.*refs/tags/v||' | sed 's/-/~/' | sort -V | tail -1 | sed 's/~/-/'
}
is_newer() { # <a> <b>: a is after b
  [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "${1/-/\~}" "${2/-/\~}" | sort -V | tail -1)" = "${1/-/\~}" ]
}
next_patch() { # <x.y.z[-pre]>
  local core=${1%%-*} major minor patch
  IFS=. read -r major minor patch <<<"$core"
  echo "$major.$minor.$((patch + 1))"
}

set_version() { # <dir> <version>
  local f
  for f in "$1/module.json" "$1/package.json" "$1/web/package.json"; do
    [ -f "$f" ] && sed -i -E "0,/\"version\": *\"[^\"]*\"/s//\"version\": \"$2\"/" "$f"
  done
  return 0
}

# Pins this folder's modules to what has been released. released.json has them all so far.
released_json="$(git rev-parse --absolute-git-dir)/cutover-released.json"
[ -f "$released_json" ] || echo '{}' > "$released_json"
record_release() { # <repository> <tag>
  node -e 'const fs=require("fs"),[,f,r,t]=process.argv;const m=JSON.parse(fs.readFileSync(f));m[r]=t;fs.writeFileSync(f,JSON.stringify(m,null,2))' "$released_json" "$1" "$2"
}
released_tag() { # <repository>
  node -e 'const fs=require("fs"),[,f,r]=process.argv;console.log(JSON.parse(fs.readFileSync(f))[r]||"")' "$released_json" "$1"
}

pin_go() { # <dir>
  local dir=$1 mod name repo tag
  [ -f "$dir/go.mod" ] || return 0
  for mod in $(grep -oE "github\.com/$OWNER/[a-z0-9]+" "$dir/go.mod" | sort -u); do
    name=${mod##*/}
    [ "$mod" = "$(awk '/^module /{print $2; exit}' "$dir/go.mod" | tr -d '\r')" ] && continue
    repo=""
    for m in "${MODULES[@]}"; do
      read -r _ r <<<"$m"
      if [ "$(printf '%s' "$r" | tr '[:upper:]' '[:lower:]')" = "$name" ]; then repo=$r; fi
    done
    [ -n "$repo" ] || die "$dir requires $mod, which isn't one of these modules"
    tag=$(released_tag "$repo")
    [ -n "$tag" ] || die "$dir requires $repo, which comes later in the order. The order in this script needs changing."
    (cd "$dir" && GOWORK=off run go mod edit -dropreplace="$mod" && GOWORK=off run go get "$mod@$tag")
  done
  (cd "$dir" && GOWORK=off run go mod tidy)
}

pin_npm() { # <dir>
  local dir=$1
  [ -f "$dir/package.json" ] || return 0
  [ -f "$dir/go.mod" ] && return 0 # a Go module's web half depends only on its peers
  node - "$dir/package.json" "$released_json" web/package.json <<'NODE'
const fs = require('fs')
const [, , file, list, appFile] = process.argv
const raw = fs.readFileSync(file, 'utf8')
const crlf = raw.includes('\r\n')
const p = JSON.parse(raw)
const released = JSON.parse(fs.readFileSync(list))
for (const [name, spec] of Object.entries(p.dependencies || {})) {
  const m = /^github:wittywilla\/([^#]+)/.exec(spec)
  if (!m) continue
  if (!released[m[1]]) throw new Error(`${file} needs ${m[1]}, which comes later in the order`)
  p.dependencies[name] = `github:wittywilla/${m[1]}#${released[m[1]]}`
}
// Building a bundle needs the bundler, in the module's own repository.
if (p.scripts && p.scripts['build:bundle'] && !(p.devDependencies && p.devDependencies.vite)) {
  const app = JSON.parse(fs.readFileSync(appFile, 'utf8'))
  p.devDependencies = { ...p.devDependencies, vite: app.devDependencies.vite }
  console.log(`   ${file}: added vite to devDependencies, for build:bundle`)
}
const out = JSON.stringify(p, null, 2) + '\n'
fs.writeFileSync(file, crlf ? out.replace(/\n/g, '\r\n') : out)
NODE
  (cd "$dir" && run npm install --package-lock-only --ignore-scripts --no-audit --no-fund --silent)
}

# Builds and tests the way a stranger's `go install` would.
check_alone() { # <dir>
  local dir=$1
  [ -f "$dir/go.mod" ] || return 0
  if grep -q '^replace' "$dir/go.mod"; then die "$dir/go.mod has replace directives, which go install refuses"; fi
  (cd "$dir" && export GOWORK=off && run go build ./... && run go vet ./... && run go test -count=1 ./...) \
    || die "$dir doesn't build or pass its tests on its own"
}

# Real run: the release workflow, on GitHub.
wait_release_real() { # <repository> <tag> <delivery> <module path or empty>
  local repo=$1 tag=$2 delivery=$3 mod=$4 id="" i assets
  if ! "$GH" release view "$tag" --repo "$OWNER/$repo" --json tagName >/dev/null 2>&1; then
    for _ in $(seq 1 30); do
      id=$("$GH" run list --repo "$OWNER/$repo" --workflow release.yml --branch "$tag" --limit 1 --json databaseId --jq '.[0].databaseId // empty')
      [ -n "$id" ] && break
      sleep 10
    done
    [ -n "$id" ] || die "$repo's release workflow didn't start for $tag. Look at its Actions tab: https://github.com/$OWNER/$repo/actions"
    echo "   waiting for https://github.com/$OWNER/$repo/actions/runs/$id"
    signs "$delivery" && echo "   if you added yourself as a required reviewer, this waits for you to approve it there"
    "$GH" run watch "$id" --repo "$OWNER/$repo" --exit-status --interval 20 >/dev/null ||
      die "$repo's release workflow failed for $tag: https://github.com/$OWNER/$repo/actions/runs/$id. Fix it, then run this again."
  fi
  assets=$("$GH" release view "$tag" --repo "$OWNER/$repo" --json assets --jq '.assets[].name')
  if signs "$delivery"; then
    grep -q 'SHA256SUMS\.txt\.sig$' <<<"$assets" || die "$repo's release $tag has no signature among its files"
    echo "   $tag is published, with its signature"
  else
    echo "   $tag is published"
  fi
  if [ -n "$mod" ]; then
    # Ask the Go module proxy now, so the next module can require it.
    for _ in $(seq 1 10); do
      GOWORK=off go list -m "$mod@$tag" >/dev/null 2>&1 && return 0
      sleep 15
    done
    die "the Go module proxy hasn't got $mod@$tag after a few minutes"
  fi
}

# Dry run: the same workflow's own commands, here, from a clean clone of the local
# repository, which shows the module stands alone.
wait_release_dry() { # <repository> <tag> <delivery> <url>
  local repo=$1 tag=$2 delivery=$3 url=$4 flow sums clone state
  clone="$work/standalone/$repo"
  state="$work/state/$repo"
  # A dry run picked up again with --work needn't repeat what passed.
  if [ -f "$state/release.json" ] && grep -q "\"tag\": \"$tag\"" "$state/release.json"; then
    echo "   rehearsed already in this folder"
    return 0
  fi
  rm -rf "$clone" "$state"
  mkdir -p "$state"
  run git clone -q --branch "$tag" "$url" "$clone" 2>/dev/null
  flow="$clone/.github/workflows"
  run python3 "$kit_dir/workflow.py" lint "$top/.github/workflows" "$flow"/*.yml
  say "$repo: its CI, from a clean clone"
  run python3 "$kit_dir/workflow.py" run "$flow/ci.yml" test --state "$state" --checkout "$clone" --ref main --repo "$OWNER/$repo"
  (cd "$clone" && rm -rf node_modules dist release)
  say "$repo: its release build, with no key and no write token"
  # Copied wrong? The tag check has to say so.
  if python3 "$kit_dir/workflow.py" run "$flow/release.yml" build --state "$state" --checkout "$clone" --ref v9.9.9 --only "The tag must match the version" >/dev/null 2>&1; then
    die "$repo's release didn't notice a tag that differs from its version"
  fi
  run python3 "$kit_dir/workflow.py" run "$flow/release.yml" build --state "$state" --checkout "$clone" --ref "$tag" --repo "$OWNER/$repo"
  if signs "$delivery"; then
    say "$repo: signing on a clean machine"
    if python3 "$kit_dir/workflow.py" run "$flow/release.yml" sign --state "$state" --ref "$tag" >/dev/null 2>&1; then
      die "$repo's release signed with no key"
    fi
    echo "   with the secret missing it stops, as it should"
    run python3 "$kit_dir/workflow.py" run "$flow/release.yml" sign --state "$state" --ref "$tag" --set-file "secrets.WMS_SIGNING_KEY=$work/module.key"
  fi
  run python3 "$kit_dir/workflow.py" run "$flow/release.yml" publish --state "$state" --ref "$tag" --repo "$OWNER/$repo"
  if signs "$delivery"; then
    sums=$(find "$state/release" -name '*-SHA256SUMS.txt' | head -1)
    [ -n "$sums" ] && [ -f "$sums.sig" ] || die "$repo's release has no signature among its files"
    "$work/bin/verify" "$public_key" "$sums" || die "$repo's signature doesn't match the key"
    (cd "$state/release" && grep -E '\.(zip|tar\.gz)$' "$sums" | sha256sum -c --quiet -) || die "$repo's files don't match its checksum list"
    echo "   $(find "$state/release" -type f | wc -l) files, signature good, checksums good"
    say "$repo: withdrawing a version twice, with a hostile reason"
    wd="$state/withdrawals"
    mkdir -p "$wd"
    for v in "${tag#v}" 0.0.1; do
      (
        export FAKE_GH_DIR="$wd" PATH="$work/bin:$PATH"
        python3 "$kit_dir/workflow.py" run "$flow/withdraw.yml" list --state "$state/w" --repo "$OWNER/$repo" --set "inputs.version=$v" --set inputs.severity=broken \
          --set "inputs.reason=Broke \"things\"; \$(touch $work/pwned) \`touch $work/pwned\`"
        python3 "$kit_dir/workflow.py" run "$flow/withdraw.yml" sign --state "$state/w" --set-file "secrets.WMS_SIGNING_KEY=$work/module.key"
        python3 "$kit_dir/workflow.py" run "$flow/withdraw.yml" publish --state "$state/w" --repo "$OWNER/$repo"
      )
    done
    list=$(find "$wd/withdrawals" -name '*-withdrawn.json')
    "$work/bin/verify" "$public_key" "$list" || die "$repo's list of withdrawn versions has a bad signature"
    grep -q '"serial": 2' "$list" || die "$repo's list of withdrawn versions didn't go on from the one before"
    [ ! -e "$work/pwned" ] || die "a withdrawal's reason ran as a command"
    grep -q -- '--prerelease --latest=false' "$wd/withdrawals.created" || die "the withdrawals release isn't a prerelease that's never latest"
    if FAKE_GH_DIR="$wd" PATH="$work/bin:$PATH" python3 "$kit_dir/workflow.py" run "$flow/withdraw.yml" list --state "$state/w" --repo "$OWNER/$repo" \
      --set 'inputs.version=-bad' --set inputs.severity=broken --set inputs.reason=x >/dev/null 2>&1; then
      die "the withdraw workflow took something that isn't a version"
    fi
    echo "   list signed, serial went up, the reason stayed text, a bad version was refused"
  fi
  (cd "$clone" && rm -rf node_modules dist release)
}

release_module() { # <dir> <repository>
  local dir=$1 repo=$2 url delivery mod="" current highest version tag lower tool had_tag="" tag_tree="" now_tree=""
  url=$(repo_url "$repo")
  delivery=$(delivery_of "$dir")
  say "$repo ($dir, delivery $delivery)"
  [ -z "$(git status --porcelain -- "$dir")" ] || die "$dir has uncommitted changes"
  [ -f "$dir/go.mod" ] && mod=$(awk '/^module /{print $2; exit}' "$dir/go.mod" | tr -d '\r')
  current=$(field version "$(version_file "$dir")")
  highest=$(newest_tag "$url")
  echo "   version here: $current; newest on $repo: ${highest:-none}"

  # The workflows use the released WittyModule. It is the first to be released, so
  # its own workflows name the version it is about to be.
  tool=$(released_tag WittyModule)
  [ -n "$tool" ] || tool=v$current

  pin_go "$dir"
  pin_npm "$dir"
  run bash scripts/module-kit.sh --tool "$tool" "$dir"
  git add -- "$dir"

  # Already released, with exactly these files? Then a run that stopped part way is
  # picking up again, and there is nothing to do for this one.
  if git ls-remote --tags --exit-code "$url" "refs/tags/v$current" >/dev/null 2>&1; then
    had_tag=1
    git fetch -q "$url" "refs/tags/v$current"
    tag_tree=$(git rev-parse 'FETCH_HEAD^{tree}')
    now_tree=$(git write-tree --prefix="$dir/")
  fi
  if [ -n "$had_tag" ] && [ "$tag_tree" = "$now_tree" ]; then
    version=$current
    echo "   v$version is already released with these files"
    if ! git diff --cached --quiet; then
      run git -c user.name="$NAME" -c user.email="$EMAIL" commit -q -m "$repo: what its repository already has"
    fi
  else
    if [ -z "$highest" ] || is_newer "$current" "$highest"; then version=$current; else version=$(next_patch "$highest"); fi
    if [ "$version" != "$current" ]; then
      echo "   raising it to $version: v$current is taken"
      set_version "$dir" "$version"
      [ -f "$dir/package-lock.json" ] && (cd "$dir" && run npm install --package-lock-only --ignore-scripts --no-audit --no-fund --silent)
      [ "$repo" = WittyModule ] && run bash scripts/module-kit.sh --tool "v$version" "$dir"
      git add -- "$dir"
    fi
    if ! git diff --cached --quiet; then
      run git -c user.name="$NAME" -c user.email="$EMAIL" commit -q -m "$repo $version: released from its own repository"
    fi
    check_alone "$dir"
  fi
  tag=v$version

  publish "$dir" "$url" main
  if [ -d "$dir/wiki" ]; then
    wiki="${url%.git}.wiki.git"
    if [ -z "$apply" ]; then git init -q --bare -b master "$work/bare/$repo.wiki.git" 2>/dev/null || true; fi
    if GIT_TERMINAL_PROMPT=0 git ls-remote "$wiki" >/dev/null 2>&1; then
      saved=$published
      publish "$dir/wiki" "$wiki" master
      published=$saved
    else
      echo "   wiki skipped: create its first page at https://github.com/$OWNER/$repo/wiki once, then publish it by hand"
    fi
  fi
  tag_release "$url" "$repo" "$published" "$tag"
  record_release "$repo" "$tag"
  if [ -n "$apply" ]; then
    wait_release_real "$repo" "$tag" "$delivery" "$mod"
  else
    wait_release_dry "$repo" "$tag" "$delivery" "$url"
  fi
  lower=$(printf '%s' "$repo" | tr '[:upper:]' '[:lower:]')
  echo "   $lower: $tag done"
}

for m in "${MODULES[@]}"; do
  read -r dir repo <<<"$m"
  release_module "$dir" "$repo"
done

# ---------------------------------------------------------------- the server

say "This repository: from module folders to released versions"
run git rm -rq -- modules web/src/modules apps/wittyreader
rm -rf modules web/src/modules apps/wittyreader
rmdir apps 2>/dev/null || echo "   apps/ stays: it holds $(ls apps | tr '\n' ' ')"
run git rm -q -- go.work go.work.sum scripts/mirror.sh scripts/release-module.sh
run node scripts/module-kit/cutover-server.mjs "$top" "$released_json"

say "go.mod names released versions, with no replace lines"
for m in "${MODULES[@]}"; do
  read -r _ repo <<<"$m"
  case " $PURE_WEB " in *" $repo "*) continue ;; esac
  mod="github.com/$OWNER/$(printf '%s' "$repo" | tr '[:upper:]' '[:lower:]')"
  run go mod edit -dropreplace="$mod" -require="$mod@$(released_tag "$repo")"
done
# The program that runs games is built from the released module, and has to be in
# go.sum along with what it needs, which the server itself doesn't import.
run go mod edit -tool="github.com/$OWNER/wittyplay/cmd/wittyplay-host"
export GOWORK=off
run go mod tidy

say "package-lock.json names released tags"
(cd web && run npm install --package-lock-only --ignore-scripts --no-audit --no-fund --silent)
(cd web && run npm ci --ignore-scripts --no-audit --no-fund --silent)
run node web/scripts/sync-module-manifests.mjs
echo "   web/module-manifests: $(ls web/module-manifests | tr '\n' ' ')"

git add -A -- . ':!web/dist'
if [ -n "$apply" ]; then
  run git -c user.name="$NAME" -c user.email="$EMAIL" commit -q -m "The modules live in their own repositories, and the server consumes released versions"
else
  run git -c user.name="$NAME" -c user.email="$EMAIL" commit -q -m "The modules live in their own repositories, and the server consumes released versions (dry run)"
fi

# ---------------------------------------------------------------- it still builds

say "The server builds from released versions, with no modules/ folder"
for gone in modules web/src/modules apps/wittyreader go.work scripts/mirror.sh; do [ ! -e "$gone" ] || die "$gone is still here"; done
if grep -q '^replace' go.mod; then die "go.mod still has replace lines"; fi
leftover=$(git grep -nE '(^|[^a-z/])modules/witty|\./modules/witty|src/modules|apps/wittyreader' -- . ':!docs' ':!CHANGELOG.md' ':!scripts/module-kit' ':!scripts/module-kit.sh' ':!scripts/cutover-modules.sh' ':!scripts/dev-modules.sh' ':!clients/wittymediaclient/docs' || true)
if [ -n "$leftover" ]; then printf '%s\n' "$leftover"; die "these still name a module's folder"; fi
go list -m -f '{{if .Replace}}REPLACED {{.Path}}{{else}}{{.Path}} {{.Version}}{{end}}' all | grep "^github.com/$OWNER/\|^REPLACED" || true
if go list -m -f '{{if .Replace}}REPLACED{{end}}' all | grep -q REPLACED; then die "something in the build is still replaced"; fi

(cd web && run npm run build)
[ -z "$(git status --porcelain -- web/module-manifests)" ] || die "the build changed web/module-manifests"
if [ -f scripts/design-tokens.py ] && command -v python3 >/dev/null 2>&1; then
  run python3 scripts/theme-contrast.py
  run python3 scripts/design-tokens.py --check
fi
run go vet ./...
run go build -o "$top/.build/wms-cutover" ./cmd/wms
run go build -o "$top/.build/wittyplay-host-cutover" "github.com/$OWNER/wittyplay/cmd/wittyplay-host"
for t in windows/amd64 linux/arm freebsd/arm64 darwin/arm64; do
  flags=()
  case $t in freebsd/*) flags=(-gcflags="github.com/ebitengine/purego/internal/fakecgo=-std") ;; esac
  say "Cross-compiling for $t"
  GOOS=${t%/*} GOARCH=${t#*/} GOARM=7 CGO_ENABLED=0 run go build -o /dev/null ./cmd/wms
  GOOS=${t%/*} GOARCH=${t#*/} GOARM=7 CGO_ENABLED=0 run go build ${flags[@]+"${flags[@]}"} -o /dev/null "github.com/$OWNER/wittyplay/cmd/wittyplay-host"
done

data=$(mktemp -d)
say "The server starts, with the web app and the manifests inside it (a test server, port 19797)"
WMS_DATA_DIR="$data" WMS_LISTEN=127.0.0.1:19797 "$top/.build/wms-cutover" serve >"$data/out.log" 2>&1 &
server=$!
started=""
for _ in $(seq 1 40); do
  if node -e 'fetch("http://127.0.0.1:19797/api/health").then(r=>process.exit(r.ok?0:1),()=>process.exit(1))' 2>/dev/null; then started=1; break; fi
  sleep 0.5
done
page=$(node -e 'fetch("http://127.0.0.1:19797/").then(r=>r.text()).then(t=>console.log(t.includes("<div")?"page":"no page"),()=>console.log("no page"))')
kill "$server" 2>/dev/null || true
wait "$server" 2>/dev/null || true
if [ -z "$started" ] || [ "$page" != page ]; then cat "$data/out.log" >&2; die "the server built from released versions didn't start and serve its page"; fi
echo "   it answered /api/health and served the app"
rm -rf "$data"

if [ -z "$apply" ]; then
  say "This repository's new CI steps, run for real"
  run python3 "$kit_dir/workflow.py" run "$top/.github/workflows/ci.yml" test --state "$work/state/server" --checkout "$top" --ref master \
    --only "Module manifests and version ranges" --only "The module manifests the server embeds match the modules in package.json" \
    --only "Build-script smoke test (no FFmpeg downloads)"
fi

say "This repository's tests"
export WMS_TEST_LAN=""
run go test ./... -count=1 -timeout 30m

if [ -z "$apply" ]; then
  say "scripts/dev-modules.sh: working on a server and modules together"
  (
    unset GOWORK
    run bash scripts/dev-modules.sh WittyPhotos WittyEbook
    [ -f go.work ] || die "dev-modules.sh made no go.work"
    [ -z "$(git status --porcelain -- go.work go.work.sum)" ] || die "git sees go.work: .gitignore doesn't cover it"
    run go build -o /dev/null ./cmd/wms
    (cd web && run npm run build)
    run bash scripts/dev-modules.sh --off
    [ ! -f go.work ] || die "dev-modules.sh --off left go.work"
    [ -z "$(git status --porcelain | grep -v '^?? \.build' || true)" ] || { git status --short; die "dev-modules.sh left changes behind"; }
  )
  rm -rf "$work/WittyPhotos" "$work/WittyEbook"
fi

# ---------------------------------------------------------------- what was proved

if [ -n "$apply" ]; then
  say "Done. The modules are released, and branch modules-cutover has the server's change"
  echo "   It has not been pushed. Next: docs/MODULES-CUTOVER.md."
  exit 0
fi

say "Dry run passed"
cat <<EOF

Published, from folder history with nothing rewritten, to local copies of each repository:
$(cat "$released_json")

Proved here:
  - Every module's folder publishes with its history, and each new tag holds it.
  - Each one, from a clean clone with no sibling folders: its CI's vet, test and every
    platform's build, its release build, signing on a machine with nothing else, and
    the checksum list and signature checked by a separate program.
  - A missing key stops a release. A wrong tag stops a release. A hostile reason in a
    withdrawal stays text, and the list's serial goes up.
  - Each workflow keeps the rules: pinned actions, no permissions by default, no
    expressions in commands, the key only where nothing else runs.
  - The server, with no modules/ folder, no go.work and no replace lines: go.mod and
    package-lock.json name those tags, the web app and every platform build, it starts
    and serves its page, and its tests pass.
  - scripts/dev-modules.sh works, and leaves nothing behind.

NOT proved, because only GitHub can:
  - That the workflows run there: this script ran their commands itself. The release
    environment holding a secret back from other jobs, approvals, and the action that
    creates a release were not exercised.
  - gh run watch and gh release view against a real run, and pushing with your sign-in.
  - The Go module proxy and checksum database taking the new tags: here Go read the
    tags directly from the local repositories.
  - npm's download of a tag from GitHub: here it cloned from the local repositories.
NOT proved, because it would need those tags on GitHub:
  - The Docker image. Its build fetches the modules from GitHub, so it can't be built
    until they are there. Only the Dockerfile's text was checked: it names no module folder.

Secrets, environments and approval settings on GitHub were not touched or checked.
EOF
if [ -z "$keep" ]; then
  trap - EXIT
  cd /
  chmod -R u+w "$work" 2>/dev/null || true
  rm -rf "$work"
else
  echo "The dry run's folder: $work"
fi
