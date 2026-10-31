#!/usr/bin/env bash
# Gives the repositories that sign releases the module key: for each one, creates the
# `release` environment if it isn't there, and sets the secret WMS_SIGNING_KEY in it, so
# only the jobs that sign can read it. The owner runs this.
#
#   scripts/set-module-secret.sh ~/.wms-signing/module.key
#   scripts/set-module-secret.sh --require-approval --protect-tags ~/.wms-signing/module.key
#                                     and each signature waits for you to approve it, and
#                                     only you can create or change a v* tag (recommended)
#   scripts/set-module-secret.sh --check
#                                     only list what each repository has (names, never values)
#
# The key's text is never printed: it goes straight from the file to the GitHub CLI, which
# encrypts it before sending. WittyModule, WittyDisc and WittyReader sign nothing, so they
# never get it.
set -euo pipefail

OWNER=${OWNER:-wittywilla}
REPOS=(WittyDVD WittyBluray WittyIntegratedFileBrowser WittyPhotos WittyTuner WittyPlay WittyInk WittyEbook WittyPDF WittyComics)

GH=gh
if ! command -v gh >/dev/null 2>&1; then
  GH="/c/Program Files/GitHub CLI/gh.exe"
  [ -x "$GH" ] || { echo "This needs the GitHub CLI: https://cli.github.com" >&2; exit 1; }
fi
"$GH" auth status >/dev/null 2>&1 || { echo "Sign in first: gh auth login" >&2; exit 1; }

check=""
approval=""
protect=""
key=""
for arg in "$@"; do
  case "$arg" in
    --check) check=1 ;;
    --require-approval) approval=1 ;;
    --protect-tags) protect=1 ;;
    -*) echo "unknown option: $arg" >&2; exit 2 ;;
    *) key=$arg ;;
  esac
done

if [ -n "$check" ]; then
  for repo in "${REPOS[@]}"; do
    env=$("$GH" api "repos/$OWNER/$repo/environments/release" --jq '[.protection_rules[]?.type] | join(",")' 2>/dev/null) || { echo "$repo: no release environment"; continue; }
    secrets=$("$GH" secret list --env release --repo "$OWNER/$repo" --json name --jq '[.[].name] | join(",")')
    tags=$("$GH" api "repos/$OWNER/$repo/rulesets" --jq '[.[] | select(.target == "tag") | .name] | join(",")' 2>/dev/null || true)
    echo "$repo: release environment, rules: ${env:-none}; secrets: ${secrets:-none}; tag rules: ${tags:-none}"
  done
  exit 0
fi

[ -n "$key" ] || { echo "usage: scripts/set-module-secret.sh [--require-approval] [--protect-tags] <key file>" >&2; exit 2; }
[ -s "$key" ] || { echo "$key is missing or empty" >&2; exit 1; }
case "$(basename "$key")" in
  release.key) echo "$key looks like the server's release key. Modules use their own: scripts/new-module-key.sh" >&2; exit 1 ;;
esac
tail -n 1 "$key" | grep -qE '^[A-Za-z0-9+/]{43}=[[:space:]]*$' || { echo "$key isn't a key made by scripts/new-module-key.sh" >&2; exit 1; }

reviewers='{}'
if [ -n "$approval" ]; then
  reviewers="{\"reviewers\":[{\"type\":\"User\",\"id\":$("$GH" api user --jq .id)}]}"
fi

for repo in "${REPOS[@]}"; do
  # Creating an environment over one that exists would clear its approval rules, so an
  # existing one changes only when asked to.
  if [ -n "$approval" ] || ! "$GH" api "repos/$OWNER/$repo/environments/release" >/dev/null 2>&1; then
    printf '%s' "$reviewers" | "$GH" api -X PUT "repos/$OWNER/$repo/environments/release" --input - >/dev/null
    echo "$repo: release environment ready"
  fi
  "$GH" secret set WMS_SIGNING_KEY --env release --repo "$OWNER/$repo" < "$key" >/dev/null
  echo "$repo: secret set"
  if [ -n "$protect" ]; then
    if "$GH" api "repos/$OWNER/$repo/rulesets" --jq '.[].name' | grep -Fxq "Version tags"; then
      echo "$repo: version tags already protected"
    else
      # Admins, which is you, may pass. Actions' own token may not.
      "$GH" api -X POST "repos/$OWNER/$repo/rulesets" --input - >/dev/null <<'JSON'
{"name":"Version tags","target":"tag","enforcement":"active",
 "conditions":{"ref_name":{"include":["refs/tags/v*"],"exclude":[]}},
 "rules":[{"type":"creation"},{"type":"update"},{"type":"deletion"},{"type":"non_fast_forward"}],
 "bypass_actors":[{"actor_id":5,"actor_type":"RepositoryRole","bypass_mode":"always"}]}
JSON
      echo "$repo: version tags protected"
    fi
  fi
done
echo
echo "Done. Check with: scripts/set-module-secret.sh --check"
