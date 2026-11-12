// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What a title's page shows under "Where to watch". Run with:
//   node --experimental-strip-types --test web/src/components/watchOffers.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { copyDetail, httpsUrl, sizeWords, watchView } from './watchOffers.ts'

// An answer in the documented shape (docs/CLIENT-API.md, Media extras), as the server's own
// test builds it: one service to stream, one shop to rent and to buy.
const local = { itemId: 7, title: 'The Film', quality: '1080p', versions: [
  { itemId: 7, kind: 'video', quality: '1080p', width: 1920, height: 1080, container: 'matroska,webm', videoCodec: 'hevc', sizeBytes: 4_200_000_000 },
  { itemId: 9, label: 'Director’s Cut', kind: 'video', quality: '720p', width: 1280, height: 720, container: 'mov', videoCodec: 'h264', sizeBytes: 900_000_000 },
] }
const keyed = {
  inLibrary: true, itemId: 7, local, region: 'GB', tmdb: { type: 'movie', tmdbId: '603' }, source: 'tmdb',
  link: 'https://www.themoviedb.org/movie/603/watch?locale=GB',
  groups: [
    { type: 'stream', providers: [{ providerId: 8, key: 'streamer', name: 'Streamer', type: 'stream', logo: 'https://image.tmdb.org/t/p/w92/a.png', priority: 1 }] },
    { type: 'rent', providers: [{ providerId: 3, key: 'shop', name: 'Shop', type: 'rent', priority: 2 }] },
    { type: 'buy', providers: [{ providerId: 3, key: 'shop', name: 'Shop', type: 'buy', logo: 'http://plain.example/logo.png', priority: 2 }] },
  ],
  providers: [], attribution: 'Listings from JustWatch, through TMDB.', fetchedAt: 1794400000, cached: false,
}
const noKey = { inLibrary: true, itemId: 7, local, region: 'US', groups: [], providers: [], reason: 'no-api-key', fetchedAt: 0, cached: false }

test('services are grouped in the order people look for them, with the link and the credit', () => {
  const v = watchView(keyed, { admin: false })
  assert.deepEqual(v.groups.map((g) => [g.type, g.title, g.providers.map((p) => p.name)]), [['stream', 'Stream', ['Streamer']], ['rent', 'Rent', ['Shop']], ['buy', 'Buy', ['Shop']]])
  assert.equal(v.groups[0].providers[0].logo, 'https://image.tmdb.org/t/p/w92/a.png')
  assert.equal(v.link, 'https://www.themoviedb.org/movie/603/watch?locale=GB')
  assert.equal(v.attribution, 'Listings from JustWatch, through TMDB.')
  assert.equal(v.region, 'GB')
  assert.equal(v.asOf, 1794400000)
  assert.equal(v.note, '')
})

test('groups come out in the fixed order whatever order they arrive in, and unknown ones are left out', () => {
  const mixed = { ...keyed, groups: [{ type: 'buy', providers: keyed.groups[2].providers }, { type: 'cinema', providers: keyed.groups[0].providers }, { type: 'ads', providers: keyed.groups[0].providers }, { type: 'free', providers: [] }] }
  assert.deepEqual(watchView(mixed, { admin: false }).groups.map((g) => g.title), ['Free with adverts', 'Buy'])
})

test('a logo or link that is not https is not used', () => {
  const v = watchView(keyed, { admin: false })
  assert.equal(v.groups[2].providers[0].logo, '')
  assert.equal(v.groups[1].providers[0].logo, '')
  assert.equal(httpsUrl('javascript:alert(1)'), '')
  assert.equal(httpsUrl('nonsense'), '')
  assert.equal(watchView({ ...keyed, link: 'http://plain.example/' }, { admin: false }).link, '')
})

test('what is here is listed with its quality and every version', () => {
  const v = watchView(keyed, { admin: false })
  assert.equal(v.quality, '1080p')
  assert.deepEqual(v.copies, [{ itemId: 7, name: 'This copy', detail: '1080p · HEVC · MATROSKA · 4.2 GB' }, { itemId: 9, name: 'Director’s Cut', detail: '720p · H.264 · MOV · 900 MB' }])
  assert.equal(copyDetail({ itemId: 1, kind: 'dvd-iso', quality: 'DVD', sizeBytes: 4_700_000_000 }), 'DVD · Disc image · 4.7 GB')
  assert.equal(sizeWords(0), '')
  assert.equal(sizeWords(25_000_000_000), '25 GB')
})

test('with no key, only an administrator is told, and nobody is shown services or a credit', () => {
  const admin = watchView(noKey, { admin: true })
  assert.equal(admin.note, 'no-key')
  assert.deepEqual(admin.groups, [])
  assert.equal(admin.attribution, '')
  const viewer = watchView(noKey, { admin: false })
  assert.equal(viewer.note, '')
  assert.equal(viewer.copies.length, 2)
  // A single copy leaves nothing to say to a viewer at all.
  const bare = { ...noKey, local: { itemId: 7, quality: '1080p', versions: [local.versions[0]] } }
  assert.equal(watchView(bare, { admin: false }), null)
  assert.equal(watchView(bare, { admin: true }).note, 'no-key')
  assert.equal(watchView(null, { admin: true }), null)
})

test('the other reasons for no services are said in their own way', () => {
  assert.equal(watchView({ ...noKey, reason: 'not-matched' }, { admin: true }).note, 'not-matched')
  assert.equal(watchView({ ...noKey, reason: 'not-matched' }, { admin: false }).note, '')
  assert.equal(watchView({ ...noKey, reason: 'unavailable' }, { admin: false }).note, 'unavailable')
  // Asked, and no service has it: said, with the credit for who was asked.
  const none = watchView({ ...keyed, groups: [], providers: [] }, { admin: false })
  assert.equal(none.note, 'nowhere')
  assert.equal(none.link, '')
  assert.equal(none.attribution, 'Listings from JustWatch, through TMDB.')
})

test('a title that is not in the library shows no copies', () => {
  const v = watchView({ ...keyed, inLibrary: false, itemId: undefined, local: undefined }, { admin: false })
  assert.deepEqual(v.copies, [])
  assert.equal(v.quality, '')
  assert.equal(v.groups.length, 3)
})
