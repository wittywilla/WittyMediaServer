#!/usr/bin/env bash
# Fails when a script is in git without its executable bit. Git on Windows doesn't keep the
# bit, so a script added or rewritten there loses it, and a release can't run what's left.
# CI runs this; run it yourself from the top of the repository.
#
#   scripts/check-executable.sh
#
# A script is a *.sh file or any file that starts with #!. To fix one, from any system:
#
#   git update-index --chmod=+x <file>
set -euo pipefail

bad=$(git ls-files -s -z | while IFS=$'\t' read -r -d '' info f; do
  case "$f" in
    *.sh) ;;
    *) head -c2 -- "$f" 2>/dev/null | grep -qa '^#!' || continue ;;
  esac
  [ "${info%% *}" = 100755 ] || echo "$f"
done)
if [ -n "$bad" ]; then
  echo "Not executable in git. Run git update-index --chmod=+x on:"
  echo "$bad"
  exit 1
fi
