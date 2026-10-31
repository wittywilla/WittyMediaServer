#!/bin/sh
# Fetches everything this program links, at the exact commits it was built and tested with:
# Moonlight's streaming core (with ENet and nanors, its own submodules), and Mbed TLS.
# In a source archive they're already there, and nothing is fetched.
set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MOONLIGHT_COMMIT=5d3c0ecca00c1bb3f587acb8060476edcf8e24aa
MBEDTLS_COMMIT=068ff080b369adfac81509f9b57b2afabaf82dc5 # mbedtls-3.6.7, the long-term branch

fetch() {
  dir="$ROOT/third_party/$1"
  if [ -f "$dir/CMakeLists.txt" ] && [ ! -e "$dir/.git" ]; then
    echo "$1 came with this source"
    return
  fi
  if [ ! -e "$dir/.git" ]; then
    mkdir -p "$dir"
    git -C "$dir" init --quiet
    git -C "$dir" remote add origin "$2"
  fi
  if [ "$(git -C "$dir" rev-parse --verify --quiet HEAD || true)" != "$3" ]; then
    git -C "$dir" fetch --quiet --depth 1 origin "$3"
    git -C "$dir" checkout --quiet --detach "$3"
  fi
  git -C "$dir" submodule update --init --recursive --quiet --depth 1
  echo "$1 at $3"
}

fetch moonlight-common-c https://github.com/moonlight-stream/moonlight-common-c.git "$MOONLIGHT_COMMIT"
fetch mbedtls https://github.com/Mbed-TLS/mbedtls.git "$MBEDTLS_COMMIT"
