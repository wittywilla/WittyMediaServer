#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Keeps a release's tracking issue, "Release <version> tracking", labelled release.
# .github/workflows/release-tracking.yml runs this; it can be run by hand too.
#
#   scripts/release-tracking.sh open <version>        make the issue from .github/release-checklist.md,
#                                                     or bring an open one up to date, keeping its ticks
#   scripts/release-tracking.sh published <version>   tick the lines the published release proves,
#                                                     say what's left, and close it when nothing is
#
# Needs gh (signed in, or GH_TOKEN) and jq. Running either twice changes nothing the second time.
#
# env:
#   DRY_RUN=1            say what would change, and change nothing
#   GITHUB_REPOSITORY=   owner/name (default: the repository this folder is a copy of)
#   TEMPLATE=            the checklist (default: .github/release-checklist.md)
set -euo pipefail

LABEL=release
MARK='<!-- release-tracking: published -->'

# run <command...>: runs it, or with DRY_RUN=1 only says it.
run() {
  if [ "${DRY_RUN:-}" = 1 ]; then echo "would run: $*"; else "$@"; fi
}

# keep_ticks <old body> <new body>: the new body, with every line ticked that the old one had ticked.
keep_ticks() {
  awk -v oldf="$1" '
    BEGIN {
      while ((getline l < oldf) > 0) {
        if (l ~ /^[[:space:]]*- \[[xX]\] /) { sub(/^[[:space:]]*- \[[xX]\] /, "", l); ticked[l] = 1 }
      }
    }
    {
      if ($0 ~ /^[[:space:]]*- \[ \] /) { k = $0; sub(/^[[:space:]]*- \[ \] /, "", k); if (k in ticked) sub(/- \[ \] /, "- [x] ") }
      print
    }' "$2"
}

# tick_published <body> <asset names>: the body, with "<!-- published -->" lines ticked, and
# "<!-- assets: -a.zip -b.zip -->" lines ticked when the release has a file ending in each.
tick_published() {
  awk -v namesf="$2" '
    BEGIN { while ((getline l < namesf) > 0) names[++n] = l }
    function has(suffix,   i, name) {
      for (i = 1; i <= n; i++) {
        name = names[i]
        if (length(name) >= length(suffix) && substr(name, length(name) - length(suffix) + 1) == suffix) return 1
      }
      return 0
    }
    {
      if ($0 ~ /^[[:space:]]*- \[ \] /) {
        if ($0 ~ /<!-- published -->/) {
          sub(/- \[ \] /, "- [x] ")
        } else if (match($0, /<!-- assets: [^>]* -->/)) {
          m = split(substr($0, RSTART + 13, RLENGTH - 17), want, " ")
          ok = (m > 0)
          for (i = 1; i <= m; i++) if (!has(want[i])) ok = 0
          if (ok) sub(/- \[ \] /, "- [x] ")
        }
      }
      print
    }' "$1"
}

# unticked <body>: what is still to do, as a list, without the notes for this script.
unticked() {
  sed -n 's/^[[:space:]]*- \[ \] \(.*\)$/- \1/p' "$1" | sed 's/[[:space:]]*<!--[^>]*-->[[:space:]]*$//'
}

# find_issue: "<number> <OPEN|CLOSED>" of the issue with exactly this title, the oldest when
# there are several, or nothing. Listed by label, not searched: a search can lag behind.
find_issue() {
  gh issue list -R "$REPO" --label "$LABEL" --state all --limit 1000 --json number,title,state |
    jq -r --arg t "$TITLE" '[.[] | select(.title == $t)] | sort_by(.number) | .[0] // empty | "\(.number) \(.state)"'
}

open_issue() {
  [ -f "$TEMPLATE" ] || { echo "no checklist at $TEMPLATE" >&2; exit 1; }
  # The version is only digits, letters, dots and dashes: safe to put in a sed script.
  sed "s/{{VERSION}}/$VERSION/g" "$TEMPLATE" > "$work/new.md"
  local found number state
  found="$(find_issue)"
  if [ -z "$found" ]; then
    # The label is there already in this repository; a fork may not have it.
    gh label list -R "$REPO" --limit 1000 --json name | jq -e --arg l "$LABEL" 'any(.[]; .name == $l)' >/dev/null ||
      run gh label create "$LABEL" -R "$REPO" --description "A release checklist" --color 5319e7
    run gh issue create -R "$REPO" --title "$TITLE" --label "$LABEL" --body-file "$work/new.md"
    echo "opened: $TITLE"
    return
  fi
  read -r number state <<<"$found"
  if [ "$state" != OPEN ]; then
    echo "#$number ($TITLE) is closed: left as it is"
    return
  fi
  gh issue view "$number" -R "$REPO" --json body --jq .body | tr -d '\r' > "$work/old.md"
  keep_ticks "$work/old.md" "$work/new.md" > "$work/merged.md"
  if diff -q "$work/old.md" "$work/merged.md" >/dev/null; then
    echo "#$number ($TITLE) is up to date"
    return
  fi
  run gh issue edit "$number" -R "$REPO" --body-file "$work/merged.md"
  echo "updated #$number ($TITLE)"
}

published() {
  local found number state url left id
  found="$(find_issue)"
  if [ -z "$found" ]; then
    echo "::notice::no issue called \"$TITLE\": nothing to tick"
    return
  fi
  read -r number state <<<"$found"
  gh release view "v$VERSION" -R "$REPO" --json assets,url,isDraft > "$work/release.json" ||
    { echo "there is no release v$VERSION" >&2; exit 1; }
  if [ "$(jq -r .isDraft "$work/release.json")" = true ]; then
    echo "::notice::v$VERSION is still a draft: nothing to tick"
    return
  fi
  url="$(jq -r .url "$work/release.json")"
  jq -r '.assets[].name' "$work/release.json" > "$work/names.txt"

  gh issue view "$number" -R "$REPO" --json body --jq .body | tr -d '\r' > "$work/old.md"
  tick_published "$work/old.md" "$work/names.txt" > "$work/ticked.md"
  if ! diff -q "$work/old.md" "$work/ticked.md" >/dev/null; then
    run gh issue edit "$number" -R "$REPO" --body-file "$work/ticked.md"
  fi

  left="$(unticked "$work/ticked.md")"
  {
    echo "$MARK"
    echo "[$VERSION]($url) is published, with $(wc -l < "$work/names.txt" | tr -d ' ') files."
    echo
    if [ -n "$left" ]; then
      echo "Still to tick, so this stays open:"
      echo
      echo "$left"
    else
      echo "Everything is ticked."
    fi
  } > "$work/comment.md"
  # One comment, changed in place when this runs again.
  id="$(gh api "repos/$REPO/issues/$number/comments" --paginate --jq '.[] | select(.body | startswith("'"$MARK"'")) | .id' | head -n 1)"
  if [ -z "$id" ]; then
    run gh issue comment "$number" -R "$REPO" --body-file "$work/comment.md"
  elif [ "$(gh api "repos/$REPO/issues/comments/$id" --jq .body | tr -d '\r')" != "$(cat "$work/comment.md")" ]; then
    run gh api -X PATCH "repos/$REPO/issues/comments/$id" -F "body=@$work/comment.md" --silent
  fi
  if [ -z "$left" ] && [ "$state" = OPEN ]; then
    run gh issue close "$number" -R "$REPO" --reason completed
    echo "closed #$number ($TITLE)"
  else
    echo "ticked #$number ($TITLE)$([ -n "$left" ] && echo ": $(printf '%s\n' "$left" | wc -l | tr -d ' ') left")"
  fi
}

main() {
  local mode="${1:-}"
  VERSION="${2:-}"
  VERSION="${VERSION#v}"
  case "$mode" in
    open|published) ;;
    *) echo "usage: scripts/release-tracking.sh open|published <version>" >&2; exit 2 ;;
  esac
  if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
    echo "\"$VERSION\" isn't a version such as 0.8.0-alpha" >&2
    exit 2
  fi
  REPO="${GITHUB_REPOSITORY:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}"
  TEMPLATE="${TEMPLATE:-$(dirname "${BASH_SOURCE[0]}")/../.github/release-checklist.md}"
  TITLE="Release $VERSION tracking"
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' EXIT
  if [ "$mode" = open ]; then open_issue; else published; fi
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then main "$@"; fi
