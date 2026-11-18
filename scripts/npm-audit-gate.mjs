// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The web packages' vulnerability gate: runs `npm audit --json` in web/ and fails on any high or
// critical advisory unless web/audit-allowlist.json lists it, with a reason and an expiry date.
// - An entry past its expiry fails again, so someone has to re-check it.
// - An allowlisted advisory that shows up through a shipped (non-dev) package fails at once.
// - An entry whose advisory no longer appears is reported, so it can be removed.
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const web = path.join(path.dirname(fileURLToPath(import.meta.url)), '..', 'web')
const allow = JSON.parse(readFileSync(path.join(web, 'audit-allowlist.json'), 'utf8')).entries
const today = new Date().toISOString().slice(0, 10)

function audit(args) {
  let out
  try {
    out = execFileSync('npm', ['audit', '--json', ...args], { cwd: web, encoding: 'utf8', shell: process.platform === 'win32' })
  } catch (e) {
    out = e.stdout // npm audit exits non-zero when it finds anything
  }
  const report = JSON.parse(out)
  if (report.error) throw new Error('npm audit failed: ' + JSON.stringify(report.error))
  const found = new Map()
  for (const v of Object.values(report.vulnerabilities || {})) {
    for (const via of v.via) {
      if (typeof via === 'object' && via.url && ['high', 'critical'].includes(via.severity)) {
        found.set(via.url.split('/').pop(), `${via.name}: ${via.title}`)
      }
    }
  }
  return found
}

const all = audit([])
const shipped = audit(['--omit=dev'])
const failures = []
for (const [id, what] of all) {
  const entry = allow.find((e) => e.id === id)
  if (!entry) failures.push(`${id} (${what}) is not allowlisted`)
  else if (entry.expires < today) failures.push(`${id} (${what}): allowlist entry expired ${entry.expires}, re-check it`)
  else if (shipped.has(id)) failures.push(`${id} (${what}) reaches a shipped package, allowlist does not apply`)
  else console.log(`allowed until ${entry.expires}: ${id} (${what}) - ${entry.reason}`)
}
for (const [id, what] of shipped) if (!all.has(id)) failures.push(`${id} (${what}) in shipped packages`)
for (const e of allow) if (!all.has(e.id)) console.log(`note: ${e.id} no longer appears, remove it from web/audit-allowlist.json`)
if (failures.length) {
  console.error('npm audit gate failed:\n- ' + failures.join('\n- '))
  process.exit(1)
}
console.log('npm audit gate passed')
