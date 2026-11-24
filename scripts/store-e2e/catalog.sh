#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# A small signed catalogue for the module store's end-to-end test (web/scripts/test-module-store.mjs),
# in three states, with a throwaway key. Two small web modules only, so it builds in a minute:
#
#   <out>/a  WittyStats at its version, and WittyVisualizer with that version withdrawn.
#   <out>/b  the same, plus a newer WittyStats, for the update.
#   <out>/c  b with a malformed entry, signed: the server refuses it, and the store must say so
#            plainly while the rest keeps working. The test also serves a garbled file as a catalogue.
#
#   scripts/store-e2e/catalog.sh <out> <folder with the module repositories>
#
# Needs Go, Node and the server's web/node_modules, like scripts/make-test-catalog.sh, which it uses.
set -euo pipefail
out=${1:?where to write the catalogues}
repos=${2:?the folder holding the module repositories}
here=$(cd "$(dirname "$0")/../.." && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)

# State a: built the usual way.
bash "$here/scripts/make-test-catalog.sh" "$out/a" "$repos" WittyStats WittyVisualizer >/dev/null
tool="$out/a/.wittymodule"
key="$out/a/test.key"
pub=$(cat "$out/a/test-key.pub")

# A next WittyStats: the same module with a higher patch number and a changelog entry of its own.
next="$out/next/WittyStats"
rm -rf "$out/next"; mkdir -p "$out/next"
cp -r "$repos/WittyStats/." "$next/"
rm -rf "$next/.git" "$next/node_modules" "$next/_wt" "$next/.claude"
ver=$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' "$next/module.json" | head -1)
IFS=. read -r ma mi pa <<<"${ver%%-*}"
nver="$ma.$mi.$((pa + 1))"
sed -i "0,/\"version\": *\"$ver\"/s//\"version\": \"$nver\"/" "$next/module.json" "$next/package.json"
printf '## %s\n\n- Store test: a newer version, to update to.\n\n' "$nver" | cat - "$next/CHANGELOG.md" >"$next/CHANGELOG.new" && mv "$next/CHANGELOG.new" "$next/CHANGELOG.md"

# State b starts as a copy of a, then lists the next version.
rm -rf "$out/b"; cp -r "$out/a" "$out/b"
(
  cd "$out/next"; work=$(mktemp -d); cp -r "$next/." "$work/"; ln -s "$here/web/node_modules" "$work/node_modules"
  if grep -q 'kit/web/build-bundle.mjs' "$work/package.json"; then (cd "$work" && node "${WITTYMODULE_DIR:-$repos/WittyModule}/kit/web/build-bundle.mjs" . >/dev/null)
  else (cd "$work" && node build-bundle.mjs "$work" "$work/dist" >/dev/null); fi
  rm -rf "$out/b/wittystats-next"
  "$tool" pack -out "$out/b/wittystats-next" -web "$work/dist" "$next" >/dev/null
  "$tool" sign -key "$key" -expect "wittystats-$nver" "$out/b/wittystats-next/wittystats-$nver-SHA256SUMS.txt" >/dev/null
  "$tool" catalog-add -catalog "$out/b/catalog.json" -base wittystats-next -pub "$pub" -changelog "$next/CHANGELOG.md" -publisher Test "$out/b/wittystats-next" >/dev/null
)

# Both states: WittyVisualizer's version withdrawn. Then each is signed again.
for s in a b; do
  node - "$out/$s/catalog.json" <<'EOF'
const fs = require('fs')
const p = process.argv[2]
const c = JSON.parse(fs.readFileSync(p, 'utf8'))
for (const m of c.modules) if (m.id === 'wittyvisualizer') for (const v of m.versions) v.withdrawn = { severity: 'broken', reason: 'Store test: it draws nothing.' }
c.serial = (c.serial || 1) + 1
fs.writeFileSync(p, JSON.stringify(c, null, 2))
EOF
  "$tool" catalog-sign -key "$key" "$out/$s/catalog.json" >/dev/null
done
# b must always be newer than a, so a server that has read a takes it.
node -e "const fs=require('fs');const a=JSON.parse(fs.readFileSync(process.argv[1]));const p=process.argv[2];const b=JSON.parse(fs.readFileSync(p));if(b.serial<=a.serial){b.serial=a.serial+1;fs.writeFileSync(p,JSON.stringify(b,null,2))}" "$out/a/catalog.json" "$out/b/catalog.json"
"$tool" catalog-sign -key "$key" "$out/b/catalog.json" >/dev/null

# State c: b with one malformed entry (no name, no versions), correctly signed. wittymodule won't
# sign that, so it's signed here with the same key, as a store with a broken tool might. A server
# refuses such a catalogue; the store has to say so plainly and everything else has to keep working.
rm -rf "$out/c"; cp -r "$out/b" "$out/c"
node - "$out/c/catalog.json" "$key" <<'EOF'
const fs = require('fs'), crypto = require('crypto')
const [p, keyFile] = process.argv.slice(2)
const c = JSON.parse(fs.readFileSync(p, 'utf8'))
c.modules.push({ id: 'wittybroken', name: '', summary: '', group: 'not-a-category', publisher: { name: '' }, versions: [] })
c.serial += 1
const data = Buffer.from(JSON.stringify(c, null, 2))
fs.writeFileSync(p, data)
const seed = Buffer.from(fs.readFileSync(keyFile, 'utf8').trim(), 'base64')
const key = crypto.createPrivateKey({ key: Buffer.concat([Buffer.from('302e020100300506032b657004220420', 'hex'), seed]), format: 'der', type: 'pkcs8' })
fs.writeFileSync(p + '.sig', crypto.sign(null, data, key).toString('base64') + '\n')
EOF
echo "catalogues in $out/a, $out/b and $out/c (WittyStats $ver, then $nver; c is malformed); key $out/a/test-key.pub"
