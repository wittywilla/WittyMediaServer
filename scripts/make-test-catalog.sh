#!/usr/bin/env bash
# Builds a catalogue of signed modules from local checkouts, with a throwaway key, for a test
# server built with -tags updatetest. Nothing here touches GitHub, and a release build of the
# server can't read what this makes: only that build tag trusts another key.
#
#   scripts/make-test-catalog.sh <output folder> <folder with the module repositories> [module...]
#
# Then serve the folder on this machine and start a test server:
#
#   python3 -m http.server --bind 127.0.0.1 8099 --directory <output folder>
#   WMS_UPDATE_KEY=$(cat <output folder>/test-key.pub) WMS_MODULE_CATALOG=http://127.0.0.1:8099/catalog.json wms-test ...
set -euo pipefail

out=${1:?where to write the catalogue}
repos=${2:?the folder holding the module repositories}
shift 2
here=$(cd "$(dirname "$0")/.." && pwd)
target="$(go env GOOS)/$(go env GOARCH)"

mods=("$@")
if [ ${#mods[@]} -eq 0 ]; then
  mods=(WittyPhotos WittyIntegratedFileBrowser WittyDVD WittyBluray WittyCD WittyTuner WittyPlay WittyCouch WittyCouchHost WittyRigs WittyInk WittyEbook WittyPDF WittyComics WittyVisualizer WittyStats)
fi

mkdir -p "$out"
out=$(cd "$out" && pwd)
tool="$out/.wittymodule"
wittymodule_dir=${WITTYMODULE_DIR:-$repos/WittyModule}
(cd "$wittymodule_dir" && go build -o "$tool" ./cmd/wittymodule)
key="$out/test.key"
if [ ! -f "$key" ]; then
  "$tool" keygen -key "$key" | tail -1 >"$out/test-key.pub"
fi
pub=$(cat "$out/test-key.pub")

rm -f "$out/catalog.json" "$out/catalog.json.sig"
for repo in "${mods[@]}"; do
  dir="$repos/$repo"
  id=$(tr 'A-Z' 'a-z' <<<"$repo")
  rel="$out/$id"
  rm -rf "$rel"
  delivery=$(sed -n 's/.*"delivery": *"\([^"]*\)".*/\1/p' "$dir/module.json" | head -1)
  webEntry=$(sed -n 's/.*"webEntry": *"\([^"]*\)".*/\1/p' "$dir/module.json" | head -1)
  built=""
  if [ "$delivery" = web ] || [ -n "$webEntry" ]; then
    # The module's own bundle builder, with the tools the server's web app already has.
    work=$(mktemp -d)
    cp -r "$dir/." "$work/"
    rm -rf "$work/node_modules" "$work/.git" "$work/.claude"
    ln -s "$here/web/node_modules" "$work/node_modules"
    if [ "$id" != wittyink ] && grep -q wittyink "$dir/package.json"; then
      mkdir -p "$work/.deps"
      rm "$work/node_modules"
      mkdir "$work/node_modules"
      for n in "$here"/web/node_modules/* "$here"/web/node_modules/.bin; do ln -s "$n" "$work/node_modules/$(basename "$n")" 2>/dev/null || true; done
      rm -rf "$work/node_modules/@wittywilla"
      mkdir -p "$work/node_modules/@wittywilla"
      ln -s "$repos/WittyInk" "$work/node_modules/@wittywilla/wittyink"
    fi
    (cd "$work" && node build-bundle.mjs "$work" "$work/dist" >/dev/null)
    built="$work/dist"
  fi
  case "$delivery" in
    web)
      "$tool" pack -out "$rel" -web "$built" "$dir" >/dev/null
      ;;
    helper | program)
      if [ -f "$dir/CMakeLists.txt" ]; then
        # Its own build, which needs more than Go: name what it made with WMS_PREBUILT.
        if [ ! -f "${WMS_PREBUILT:-/nowhere}/$repo/.built" ]; then
          echo "skipping $repo: it has its own build" >&2
          continue
        fi
        "$tool" pack -out "$rel" -prebuilt "$WMS_PREBUILT/$repo" "$dir" >/dev/null
      elif [ -n "$built" ]; then
        "$tool" pack -out "$rel" -targets "$target" -web "$built" "$dir" >/dev/null
      else
        "$tool" pack -out "$rel" -targets "$target" "$dir" >/dev/null
      fi
      ;;
    *)
      echo "skipping $repo: nothing to install" >&2
      continue
      ;;
  esac
  ver=$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' "$dir/module.json" | head -1)
  "$tool" sign -key "$key" -expect "$id-$ver" "$rel/$id-$ver-SHA256SUMS.txt" >/dev/null
  "$tool" catalog-add -catalog "$out/catalog.json" -base "$id" -pub "$pub" -changelog "$dir/CHANGELOG.md" -publisher "Test" "$rel"
done
"$tool" catalog-sign -key "$key" "$out/catalog.json" >/dev/null
"$tool" catalog-check -pub "$pub" "$out/catalog.json"
