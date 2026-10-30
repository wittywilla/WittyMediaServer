#!/usr/bin/env bash
# Builds, signs and publishes a plugin's release from this machine, the same way
# the .github/workflows/release.yml in its own repository does when a tag lands
# there. See docs/MODULES.md, "Releasing a plugin".
#
#   scripts/release-module.sh modules/wittyphotos v0.2.0
#   DRY_RUN=1 scripts/release-module.sh modules/wittyphotos v0.2.0   stop before publishing
#
# The tag must already be on the plugin's repository (scripts/mirror.sh makes it).
# The signing key comes from WMS_SIGNING_KEY or ~/.wms-signing/release.key, and
# nothing is published without it. Publishing needs the GitHub CLI (gh).
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

dir=${1:-}
tag=${2:-}
[ -n "$dir" ] && [ -f "$dir/module.json" ] || { echo "usage: scripts/release-module.sh <plugin folder> <vX.Y.Z>" >&2; exit 1; }
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "not a version tag: $tag (want vX.Y.Z)" >&2; exit 1; }

field() { sed -n "s/.*\"$1\": *\"\([^\"]*\)\".*/\1/p" "$dir/module.json" | head -1; }
id=$(field id)
version=$(field version)
repo=$(field repo)
[ "v$version" = "$tag" ] || { echo "$dir/module.json says $version, and the tag is $tag" >&2; exit 1; }
if ! git diff --quiet HEAD -- "$dir" || [ -n "$(git ls-files --others --exclude-standard -- "$dir")" ]; then
  echo "$dir has uncommitted changes; commit them first" >&2; exit 1
fi

out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
tool() { go run ./modules/wittymodule/cmd/wittymodule "$@"; }
tool check "$dir"
tool pack -out "$out" "$dir"
tool sign "$out/$id-$version-SHA256SUMS.txt"

if [ -n "${DRY_RUN:-}" ]; then
  ls -l "$out"
  echo "DRY_RUN: nothing was published"
  exit 0
fi
flags=()
case "$tag" in *-*) flags+=(--prerelease) ;; esac
gh release create "$tag" "$out"/* --repo "$repo" --title "$(field name) $version" --generate-notes "${flags[@]}"
