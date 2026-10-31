#!/bin/sh
# Makes the source archive each release carries, with everything a binary was built from, as
# GPL-3.0 section 6 asks: this program and every pinned dependency, ready to build with no network.
#
#   sh scripts/source-archive.sh <version> <output.tar.gz>
set -e
[ $# -eq 2 ] || { echo "usage: sh scripts/source-archive.sh <version> <output.tar.gz>" >&2; exit 1; }
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
name="wittymoonlight-host-$1-source"
out="$(cd "$(dirname "$2")" && pwd)/$(basename "$2")"

sh "$ROOT/scripts/fetch-core.sh"
# The commits are only known while the git folders are here, and the archive leaves those out.
for dep in moonlight-common-c mbedtls; do
  if [ -e "$ROOT/third_party/$dep/.git" ]; then
    echo "$dep $(git -C "$ROOT/third_party/$dep" rev-parse HEAD)"
    git -C "$ROOT/third_party/$dep" submodule status --recursive | sed -e "s|^[ +-]*\([0-9a-f]*\) \([^ ]*\).*|$dep/\2 \1|"
  fi
done > "$ROOT/third_party/COMMITS.txt"

tar -C "$ROOT" --exclude=.git --exclude=./build --exclude=./dist --sort=name --owner=0 --group=0 --numeric-owner \
  --transform "s|^\.|$name|" -czf "$out" .
echo "$out"
