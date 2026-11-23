#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Builds what the smoke tests run, from this checkout: the server with its web app (with the
# updatetest tag, so it trusts a throwaway catalogue key), Wittyconnect, a catalogue holding
# WittyPlay and WittyTuner, and WittyRelay.
#
#   scripts/smoke/build.sh <output folder> [folder with module checkouts]
#
# Module repositories missing from that folder are cloned at SMOKE_MODULE_REF (the release branch
# this checkout targets, else main). Writes <out>/bin/{wittymediaserver,wittyconnect,wittyrelay}
# and <out>/catalog. SMOKE_SKIP_WEB=1 uses web/dist as it is.
set -euo pipefail
out=${1:?where to build}
here=$(cd "$(dirname "$0")/../.." && pwd)
mkdir -p "$out/bin"
out=$(cd "$out" && pwd)
repos=${2:-$out/repos}
mkdir -p "$repos"
repos=$(cd "$repos" && pwd)
ref=${SMOKE_MODULE_REF:-}

clone() {
  local r=$1
  [ -d "$repos/$r/.git" ] || [ -f "$repos/$r/go.mod" ] && return 0
  local b=$ref
  if [ -n "$b" ] && ! git ls-remote --exit-code --heads "https://github.com/wittywilla/$r" "$b" >/dev/null 2>&1; then b=; fi
  git clone -q --depth 1 ${b:+--branch "$b"} "https://github.com/wittywilla/$r" "$repos/$r"
  echo "$r: $(git -C "$repos/$r" log --oneline -1)"
}
for r in WittyModule WittyPlay WittyTuner WittyRelay; do clone "$r"; done

if [ "${SMOKE_SKIP_WEB:-}" != 1 ]; then
  (cd "$here/web" && { [ -d node_modules ] || npm ci --ignore-scripts --no-audit --no-fund; } && npm run build)
fi
(cd "$here" && go build -tags updatetest -o "$out/bin/wittymediaserver" ./cmd/wittymediaserver)
(cd "$here" && go build -o "$out/bin/wittyconnect" ./cmd/wittyconnect)
(cd "$repos/WittyRelay" && GOWORK=off go build -o "$out/bin/wittyrelay" ./cmd/wittyrelay)

# The catalogue is the slow part (WittyPlay's helpers): built again only when a module changed.
stamp=$(for r in WittyModule WittyPlay WittyTuner; do git -C "$repos/$r" rev-parse HEAD 2>/dev/null || echo "$r"; done | sha256sum | cut -c1-16)
if [ "$(cat "$out/catalog/.stamp" 2>/dev/null)" != "$stamp" ]; then
  rm -rf "$out/catalog"
  bash "$here/scripts/make-test-catalog.sh" "$out/catalog" "$repos" WittyPlay WittyTuner
  echo "$stamp" >"$out/catalog/.stamp"
fi
ls -la "$out/bin" "$out/catalog"
