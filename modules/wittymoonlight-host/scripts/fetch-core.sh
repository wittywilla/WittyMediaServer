#!/bin/sh
# Fetches Moonlight's streaming core at the exact commit this program was built and tested with.
set -e
COMMIT=5d3c0ecca00c1bb3f587acb8060476edcf8e24aa
DIR="$(dirname "$0")/../third_party/moonlight-common-c"
if [ ! -d "$DIR/.git" ]; then
  mkdir -p "$(dirname "$DIR")"
  git clone --quiet https://github.com/moonlight-stream/moonlight-common-c.git "$DIR"
fi
git -C "$DIR" fetch --quiet origin "$COMMIT" 2>/dev/null || true
git -C "$DIR" checkout --quiet "$COMMIT"
git -C "$DIR" submodule update --init --recursive --quiet
echo "moonlight-common-c at $COMMIT"
