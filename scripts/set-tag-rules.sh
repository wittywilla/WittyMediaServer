#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Makes version tags work the way docs/PUBLISHING.md says: a version tag (v*, and a Go module
# folder's <folder>/v*) is made only by each repository's Release workflow (.github/workflows/tag.yml),
# and nobody, the owner included, can move or delete one. The owner runs this.
#
#   scripts/set-tag-rules.sh                 every repository that releases
#   scripts/set-tag-rules.sh WittyCD         just these
#   scripts/set-tag-rules.sh --check [repo]  only say what each has (names, never key text)
#
# For each repository it:
#   1. makes a `tag` environment, and in it the secret TAG_DEPLOY_KEY: the private half of a new
#      deploy key with write access, made here, sent straight to GitHub, and never kept;
#   2. sets two tag rulesets, with no exception for admins:
#        "Version tags are permanent"   no update, deletion or force-push, for anyone;
#        "Version tags come from Release" no creation, except by a deploy key (only the workflow has one);
#   3. then deletes the old "Version tags" ruleset, which let admins (and so any script on the
#      owner's computer) do all of that.
# A tag pushed from a computer is then refused. Run it again at any time: what is right is left alone.
set -euo pipefail

OWNER=wittywilla
GH=${GH:-gh}
REPOS=(WittyModule WittyDisc WittyDVD WittyBluray WittyCD WittyTuner WittyPhotos WittyIntegratedFileBrowser WittyRigs
  WittyPlay WittyCouch WittyCouchHost WittyInk WittyEbook WittyPDF WittyComics WittyVisualizer WittyStats
  WittyReader WittyMediaPlayer WittyMediaClient WittyMediaServer)
KEY_TITLE="Release workflow: version tags"
PERMANENT="Version tags are permanent"
CREATION="Version tags come from Release"
INCLUDE='["refs/tags/v*","refs/tags/*/v*"]'

check=""
picked=()
for arg in "$@"; do
  case "$arg" in
    --check) check=1 ;;
    -*) sed -n '4,20p' "$0"; exit 2 ;;
    *) picked+=("$arg") ;;
  esac
done
[ ${#picked[@]} -gt 0 ] && REPOS=("${picked[@]}")
command -v "$GH" >/dev/null && "$GH" auth status >/dev/null 2>&1 || { echo "needs gh, signed in: gh auth login" >&2; exit 1; }

ruleset_id() { # <repo> <name>
  "$GH" api "repos/$OWNER/$1/rulesets" --jq ".[] | select(.target == \"tag\" and .name == \"$2\") | .id" 2>/dev/null | head -1
}

put_ruleset() { # <repo> <name> <rules json> <bypass json>
  local id body
  body=$(printf '{"name":"%s","target":"tag","enforcement":"active","conditions":{"ref_name":{"include":%s,"exclude":[]}},"rules":%s,"bypass_actors":%s}' "$2" "$INCLUDE" "$3" "$4")
  id=$(ruleset_id "$1" "$2")
  if [ -n "$id" ]; then
    "$GH" api -X PUT "repos/$OWNER/$1/rulesets/$id" --input - >/dev/null <<<"$body"
  else
    "$GH" api -X POST "repos/$OWNER/$1/rulesets" --input - >/dev/null <<<"$body"
  fi
}

if [ -n "$check" ]; then
  for repo in "${REPOS[@]}"; do
    rules=$("$GH" api "repos/$OWNER/$repo/rulesets" --jq '[.[] | select(.target == "tag") | .name] | join("; ")' 2>/dev/null || echo "?")
    key=$("$GH" api "repos/$OWNER/$repo/keys" --jq "[.[] | select(.title == \"$KEY_TITLE\") | if .read_only then \"read-only\" else \"write\" end] | join(\",\")" 2>/dev/null || echo "?")
    secret=$("$GH" secret list --env tag --repo "$OWNER/$repo" --json name --jq '[.[].name] | join(",")' 2>/dev/null || echo "no tag environment")
    workflow=$("$GH" api "repos/$OWNER/$repo/contents/.github/workflows/tag.yml" --jq .name 2>/dev/null) || workflow="not on the default branch"
    echo "$repo: tag rules: ${rules:-none}; deploy key: ${key:-none}; tag secrets: ${secret:-none}; tag.yml: $workflow"
  done
  exit 0
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
for repo in "${REPOS[@]}"; do
  "$GH" api -X PUT "repos/$OWNER/$repo/environments/tag" >/dev/null
  have_key=$("$GH" api "repos/$OWNER/$repo/keys" --jq "[.[] | select(.title == \"$KEY_TITLE\") | .id] | first // empty")
  have_secret=$("$GH" secret list --env tag --repo "$OWNER/$repo" --json name --jq '.[].name' | grep -Fx TAG_DEPLOY_KEY || true)
  if [ -z "$have_key" ] || [ -z "$have_secret" ]; then
    # A key whose private half is gone is useless: replace it.
    [ -n "$have_key" ] && "$GH" api -X DELETE "repos/$OWNER/$repo/keys/$have_key" >/dev/null
    rm -f "$work/key" "$work/key.pub"
    ssh-keygen -q -t ed25519 -N "" -C "release-tags@$repo" -f "$work/key"
    "$GH" api "repos/$OWNER/$repo/keys" -f title="$KEY_TITLE" -f key="$(cat "$work/key.pub")" -F read_only=false >/dev/null
    "$GH" secret set TAG_DEPLOY_KEY --env tag --repo "$OWNER/$repo" < "$work/key" >/dev/null
    rm -f "$work/key" "$work/key.pub"
    echo "$repo: deploy key made, its private half is in the tag environment"
  fi
  put_ruleset "$repo" "$PERMANENT" '[{"type":"update"},{"type":"deletion"},{"type":"non_fast_forward"}]' '[]'
  put_ruleset "$repo" "$CREATION" '[{"type":"creation"}]' '[{"actor_id":null,"actor_type":"DeployKey","bypass_mode":"always"}]'
  old=$(ruleset_id "$repo" "Version tags")
  [ -z "$old" ] || "$GH" api -X DELETE "repos/$OWNER/$repo/rulesets/$old" >/dev/null
  echo "$repo: version tags made only by Release, and never moved or deleted"
done
echo
echo "Done. Check with: scripts/set-tag-rules.sh --check"
