#!/usr/bin/env node
// Checks the hand-written sample responses in shared/fixtures against a running
// server, so the native apps' tests can't drift from what servers really send.
// It compares field names and types, never values, and prints no media titles.
//
//   node shared/scripts/check-fixtures.mjs http://127.0.0.1:9797 <token>
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const [server, token] = process.argv.slice(2)
if (!server || !token) {
  console.error('usage: check-fixtures.mjs <server> <token>')
  process.exit(2)
}
const dir = join(dirname(fileURLToPath(import.meta.url)), '..', 'fixtures')
const fixture = (name) => JSON.parse(readFileSync(join(dir, `${name}.json`), 'utf8'))
const live = async (path, body) => {
  const res = await fetch(server + '/api/v1' + path, {
    method: body ? 'POST' : 'GET',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) throw new Error(`${path}: ${res.status}`)
  return res.json()
}

const problems = []
const kind = (v) => (v === null ? 'null' : Array.isArray(v) ? 'array' : typeof v)

// Everything a fixture claims must exist on the live answer with a compatible
// type. Null and a missing optional field are both things apps must accept.
function compare(where, sample, real) {
  if (sample === null || real === null || real === undefined) return
  if (kind(sample) !== kind(real)) { problems.push(`${where}: sample is ${kind(sample)}, server sent ${kind(real)}`); return }
  if (Array.isArray(sample)) { if (sample.length && real.length) compare(`${where}[]`, sample[0], real[0]); return }
  if (typeof sample !== 'object') return
  for (const key of Object.keys(sample)) {
    if (!(key in real)) { if (!OPTIONAL.has(key)) problems.push(`${where}.${key}: not sent by the server`); continue }
    compare(`${where}.${key}`, sample[key], real[key])
  }
}
// Fields the server leaves out when they don't apply (Go omitempty).
const OPTIONAL = new Set(['year', 'overview', 'tagline', 'rating', 'contentRating', 'studio', 'progress', 'title', 'channels', 'libraryId', 'items', 'shows',
  'showId', 'showTitle', 'season', 'episode', 'episodeEnd', 'nextId', 'introStart', 'introEnd', 'lastScanAt', 'genres', 'subLangs', 'audioLangs', 'updatedAt',
  'intro', 'reasons', 'sdh', 'default', 'languageName', 'text', 'delivery', 'url', 'language', 'resumeSec'])

const home = await live('/home')
const libraries = home.libraries || []
const shows = libraries.find((l) => l.type === 'shows')
const items = await live('/items?limit=1&kind=video')
if (!items.items.length) throw new Error('the server needs at least one video')

compare('status', fixture('status'), await live('/status'))
compare('profiles', fixture('profiles'), await live('/profiles'))
compare('home', fixture('home'), home)
compare('playback', fixture('playback'), await live(`/items/${items.items[0].id}/playback`, { profile: 'androidtv' }))
compare('capabilities', fixture('capabilities'), await live('/capabilities'))
compare('pair-start', fixture('pair-start'), await live('/pair/start', { device: 'fixture check', platform: 'test' }))
if (shows) {
  const list = await live(`/shows?library=${shows.id}`)
  if (list.shows.length) compare('show', fixture('show'), await live(`/shows/${list.shows[0].id}`))
  else problems.push('no show on the server to compare with')
} else {
  problems.push('no shows library on the server to compare with')
}

if (problems.length) {
  console.error(problems.join('\n'))
  process.exit(1)
}
console.log('sample responses match the server')
