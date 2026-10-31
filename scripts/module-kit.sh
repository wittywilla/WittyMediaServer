#!/usr/bin/env bash
# Writes what a module's own repository needs into its folder, from the templates in
# scripts/module-kit/templates: its workflows, Dependabot's settings, a .gitignore, and
# a section of its README. Safe to run again; what it writes depends only on the folder.
#
#   scripts/module-kit.sh                      every module in this repository
#   scripts/module-kit.sh modules/wittyphotos  one folder (or a clone of its repository)
#   scripts/module-kit.sh --tool v0.3.0 ...    the WittyModule version workflows use
#
# How a release works comes from `delivery` in the folder's module.json, read each time:
#   helper, program   a program per platform, signed
#   web               a bundle for the browser, signed
#   library, builtin  no files of its own to install: tests and a release with no assets
#   (no module.json)  an app, such as WittyReader: its built web app on the release
#
# Only the versions a release can be installed from need a signing key, so only they
# get withdraw.yml. A repository that doesn't sign anything never holds the key.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
templates="$here/module-kit/templates"
FOLDERS=(
  modules/wittymodule modules/wittydisc modules/wittydvd modules/wittybluray modules/wittycd
  modules/wittyintegratedfilebrowser modules/wittyphotos modules/wittytuner modules/wittyplay modules/wittyrigs modules/wittycouch
  web/src/modules/wittyink web/src/modules/wittyebook web/src/modules/wittypdf web/src/modules/wittycomics web/src/modules/wittyvisualizer
  apps/wittyreader
)

tool_version=${WITTYMODULE_VERSION:-}
if [ "${1:-}" = "--tool" ]; then
  tool_version=${2:?--tool needs a version such as v0.3.0}
  shift 2
fi
root=$(cd "$here/.." && pwd)
if [ -z "$tool_version" ]; then
  # Before the modules left this repository, its folder says. After, go.mod does.
  if [ -f "$root/modules/wittymodule/module.json" ]; then
    tool_version=v$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' "$root/modules/wittymodule/module.json" | head -1)
  else
    tool_version=$(awk '$1 == "github.com/wittywilla/wittymodule" {print $2; exit}' "$root/go.mod")
  fi
fi
[[ "$tool_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "not a WittyModule version: \"$tool_version\"" >&2; exit 1; }

# The first "name": "value" in a JSON file, without needing a JSON tool.
field() { sed -n "s/.*\"$1\": *\"\([^\"]*\)\".*/\1/p" "$2" | head -1 | tr -d '\r'; }

# Keeps the lines between "#@if <word>..." and "#@end", which can nest, when any word is
# switched on, and fills in the @@NAMES@@. Everything else is copied.
render() { # <template> <output>
  awk -v on=" $words " '
    /^#@if / { k = 0; for (i = 2; i <= NF; i++) if (index(on, " " $i " ")) k = 1; depth++; keep[depth] = k && (depth == 1 || keep[depth - 1]); next }
    /^#@end/ { depth--; next }
    depth == 0 || keep[depth] { print }
  ' <(tr -d '\r' < "$1") | sed \
    -e "s|@@ID@@|$id|g" -e "s|@@NAME@@|$name|g" -e "s|@@VERSION_FILE@@|$version_file|g" \
    -e "s|@@TOOL@@|$tool|g" -e "s|@@TOOL_RELEASED@@|$tool_released|g" -e "s|@@PACK_FLAGS@@|$pack_flags|g" \
    -e "s|@@README_WHAT@@|$readme_what|" -e "s|@@README_VERSION@@|$readme_version|" -e "s|@@README_AFTER@@|$readme_after|" \
    -e "s|@@README_WITHDRAW@@|$readme_withdraw|" > "$2"
}

# Puts the generated section at the end of the README, or in place of the one there.
readme() { # <folder>
  local file="$1/README.md" section kept
  section=$(mktemp)
  kept=$(mktemp)
  render "$templates/readme.md" "$section"
  [ -f "$file" ] || printf '# %s\n' "$name" > "$file"
  # Without the section and the blank line before it, the file is what its owner wrote.
  awk '/<!-- module-kit -->/ { skip = 1 } !skip { print } /<!-- \/module-kit -->/ { skip = 0 }' "$file" |
    sed -e ':a' -e '/^[[:space:]]*$/{$d;N;ba' -e '}' > "$kept"
  { cat "$kept"; echo; cat "$section"; } > "$file"
  rm -f "$section" "$kept"
}

# A Go module with a web half is also an npm package, so the app can depend on a tag of
# it. Only the web half is in the package: the Go source would make every install larger.
web_package() { # <folder>
  local file="$1/package.json" description
  [ -f "$1/go.mod" ] && [ -f "$1/web/index.ts" ] || return 0
  if [ -f "$file" ]; then
    sed -i -E "0,/\"version\": *\"[^\"]*\"/s//\"version\": \"$version\"/" "$file"
    return 0
  fi
  description=$(field description "$1/web/package.json")
  cat > "$file" <<EOF
{
  "name": "@wittywilla/$id",
  "version": "$version",
  "description": "$description",
  "license": "Apache-2.0",
  "author": "WittyWilla (https://github.com/wittywilla)",
  "type": "module",
  "main": "./web/index.ts",
  "exports": {
    ".": "./web/index.ts"
  },
  "files": ["web", "LICENSE", "NOTICE"],
  "repository": "github:$repo",
  "peerDependencies": {
    "react": ">=18",
    "react-dom": ">=18"
  }
}
EOF
}

kit() { # <folder>
  local dir="$1" delivery gen="$1/.github/workflows"
  if [ -f "$dir/module.json" ]; then
    id=$(field id "$dir/module.json")
    name=$(field name "$dir/module.json")
    repo=$(field repo "$dir/module.json")
    version=$(field version "$dir/module.json")
    delivery=$(field delivery "$dir/module.json")
    version_file=module.json
    readme_version='`module.json`'
  else
    id=$(field name "$dir/package.json")
    name=$(field repository "$dir/package.json")
    name=${name##*/}
    # An app has its name in package.json only in lower case.
    [ -n "$name" ] || name="WittyReader"
    repo="wittywilla/$name"
    version=$(field version "$dir/package.json")
    delivery=app
    version_file=package.json
  fi
  [ -n "$id" ] && [ -n "$version" ] || { echo "$dir: no id or version found" >&2; exit 1; }

  words=""
  [ -f "$dir/go.mod" ] && words="$words go"
  [ "$delivery" != app ] && words="$words manifest"
  case "$delivery" in
    helper | program) words="$words signed programs" ;;
    web) words="$words signed bundle npm node" ;;
    library | builtin) words="$words source" ;;
    app) words="$words app npm node" ;;
    *) echo "$dir: unknown delivery \"$delivery\"" >&2; exit 1 ;;
  esac
  # A web module that is still built in has no bundle yet, but CI still installs it.
  if [ "$delivery" = builtin ] && [ ! -f "$dir/go.mod" ]; then words="$words npm node"; fi
  if [ -f "$dir/go.mod" ] && grep -q 'github.com/ebitengine/purego' "$dir/go.mod"; then
    words="$words purego"
    pack_flags=' -freebsd-gcflags "github.com/ebitengine/purego/internal/fakecgo=-std"'
  else
    pack_flags=''
  fi
  tool="github.com/wittywilla/wittymodule/cmd/wittymodule@$tool_version"
  tool_released=$tool
  # WittyModule checks itself with the tool it holds.
  [ "$id" = wittymodule ] && tool=./cmd/wittymodule

  readme_withdraw=""
  case "$delivery" in
    helper | program)
      readme_what="A version tag builds a program for every platform, signs the checksum list, and publishes a release that servers can install."
      readme_after="The signing step uses the \`WMS_SIGNING_KEY\` secret of the \`release\` environment, and nothing is published without it."
      ;;
    web)
      readme_what="A version tag builds the bundle for the browser, signs the checksum list, and publishes a release that servers can install."
      readme_after="The signing step uses the \`WMS_SIGNING_KEY\` secret of the \`release\` environment, and nothing is published without it."
      ;;
    library | builtin)
      readme_what="$name reaches a server inside the projects built with it, so a release has no files of its own: a version tag runs the tests and publishes a release that those projects can require."
      readme_after="There is nothing to sign."
      ;;
    app)
      readme_what="A version tag builds the web app and attaches it to a release. It isn't delivered through a server."
      readme_version='`package.json`'
      readme_after="There is nothing to sign."
      ;;
  esac
  case "$delivery" in
    helper | program | web)
      readme_withdraw="\\
To take a version back, run the *withdraw* workflow from the *Actions* tab. It adds the version to a signed list on the release tagged \`withdrawals\`, which servers read.\\
"
      ;;
  esac

  mkdir -p "$gen"
  render "$templates/ci.yml" "$gen/ci.yml"
  render "$templates/release.yml" "$gen/release.yml"
  case " $words " in
    *" signed "*) render "$templates/withdraw.yml" "$gen/withdraw.yml" ;;
    *) rm -f "$gen/withdraw.yml" ;;
  esac
  render "$templates/dependabot.yml" "$dir/.github/dependabot.yml"
  [ -f "$dir/.gitignore" ] || cp "$templates/gitignore" "$dir/.gitignore"
  web_package "$dir"
  readme "$dir"
  echo "module-kit: $dir ($delivery, WittyModule $tool_version)"
}

if [ $# -eq 0 ]; then
  cd "$root"
  set -- "${FOLDERS[@]}"
fi
for folder in "$@"; do
  folder=${folder%/}
  [ -f "$folder/module.json" ] || [ -f "$folder/package.json" ] || { echo "$folder has no module.json or package.json" >&2; exit 1; }
  kit "$folder"
done
