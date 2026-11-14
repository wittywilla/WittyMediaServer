// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Web host API 5: what moduleHost.ts gives a module (hostFor) is what WittyModule's
// kit/web/host/index.d.ts promises, name for name. A name only one side has is a bundle
// that breaks, or a promise nobody keeps. Skipped without a WittyModule checkout beside this one.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const kit = [process.env.WITTYMODULE, join(here, '../../../WittyModule'), join(here, '../../../../WittyModule')]
  .filter(Boolean).map((d) => join(d, 'kit/web/host/index.d.ts')).find((f) => existsSync(f))

// The keys of the object literal `const host = { ... }` in hostFor, at its top level.
function hostKeys(src) {
  const start = src.indexOf('const host = {')
  assert.ok(start >= 0, 'moduleHost.ts has no hostFor host object')
  let depth = 0
  let i = src.indexOf('{', start)
  const keys = []
  let lineDepth = 0
  for (let line = ''; i < src.length; i++) {
    const c = src[i]
    if (c === '\n') {
      const m = lineDepth === 1 && /^\s*([a-zA-Z]+)\s*[:,]/.exec(line)
      if (m) keys.push(m[1])
      line = ''
      lineDepth = depth
      continue
    }
    line += c
    if (c === '{' || c === '(' || c === '[') depth++
    if (c === '}' || c === ')' || c === ']') { depth--; if (depth === 0) break }
  }
  return keys.sort()
}

// The members of `export interface ModuleHost { ... }`.
function interfaceKeys(src) {
  const body = /export interface ModuleHost \{([\s\S]*?)\n\}/.exec(src)
  assert.ok(body, 'the kit has no ModuleHost')
  return [...body[1].matchAll(/^ {2}([a-zA-Z]+)[?(:]/gm)].map((m) => m[1]).sort()
}

test('the host a module is given is the one WittyModule kit/web/host promises', { skip: !kit && 'no WittyModule checkout beside this one' }, () => {
  const ours = hostKeys(readFileSync(join(here, 'moduleHost.ts'), 'utf8'))
  const theirs = interfaceKeys(readFileSync(kit, 'utf8'))
  assert.deepEqual(ours, theirs)
})
