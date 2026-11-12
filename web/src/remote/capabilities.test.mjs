// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// node --experimental-strip-types --test web/src/remote/capabilities.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  accepts, canPlayOn, checkQueue, commandsOf, deviceFeatures, errorText, featuresOf, isNavKey, keyboardKey, mediaKindsFor, playerFor,
  queueFamily, refusal, remoteControls, routeFor, screenFor, statusWords, stepWords, watchingTv,
} from './capabilities.ts'

// The protocol's own lists (internal/wittyremote/protocol.go).
const MEDIA_KINDS = ['video', 'audio', 'dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder', 'cd-image', 'book', 'radio', 'game', 'photo']
const FEATURES = ['seek', 'volume', 'queue', 'audio', 'subtitles', 'navigate', 'rate']
const NAV_KEYS = ['up', 'down', 'left', 'right', 'select', 'back', 'home', 'menu', 'playpause']

const lib = (type, available, modules = []) => ({ type, available, modules: modules.map(([id, installed]) => ({ id, name: id, installed })) })
const everything = [
  lib('movies', true, [['wittyvideo', true]]), lib('music', true, [['wittyaudio', true], ['wittycd', true]]), lib('radio', true, [['wittyaudio', true]]),
  lib('discs', true, [['wittydvd', true], ['wittybluray', true], ['wittycd', true]]), lib('books', true, [['wittyebook', true], ['wittypdf', false], ['wittycomics', false]]),
  lib('games', true, [['wittyplay', true]]), lib('photos', true, [['wittyphotos', true]]),
]

test('every kind of item the protocol knows has a player here', () => {
  for (const k of MEDIA_KINDS) assert.ok(playerFor(k), k)
  assert.equal(playerFor('dvd-iso'), 'disc')
  assert.equal(playerFor('bluray-folder'), 'disc')
  assert.equal(playerFor('cd-image'), 'cd')
  assert.equal(playerFor('audio'), 'audio')
  assert.equal(playerFor('hologram'), null)
})

test('a server with every module has every kind claimed, in the protocol\'s order', () => {
  assert.deepEqual(mediaKindsFor(everything), MEDIA_KINDS)
  assert.deepEqual(deviceFeatures(mediaKindsFor(everything)), FEATURES)
})

test('a kind whose module is missing is not claimed', () => {
  const plain = [
    lib('movies', true, [['wittyvideo', true]]), lib('music', true, [['wittyaudio', true], ['wittycd', false]]), lib('radio', true, [['wittyaudio', true]]),
    lib('discs', false, [['wittydvd', false], ['wittybluray', false], ['wittycd', false]]), lib('books', false, [['wittyebook', false], ['wittypdf', false], ['wittycomics', false]]),
    lib('games', false, [['wittyplay', false]]), lib('photos', false, [['wittyphotos', false]]),
  ]
  assert.deepEqual(mediaKindsFor(plain), ['video', 'audio', 'radio'])
  // DVDs without Blu-rays: only the DVD kinds.
  const dvdOnly = [...plain.slice(0, 3), lib('discs', true, [['wittydvd', true], ['wittybluray', false], ['wittycd', false]])]
  assert.deepEqual(mediaKindsFor(dvdOnly), ['video', 'audio', 'dvd-iso', 'dvd-folder', 'radio'])
  // One reader is enough for books.
  assert.ok(mediaKindsFor([...plain, lib('books', true, [['wittypdf', true]])]).includes('book'))
  // No music library kind at all: no audio.
  assert.deepEqual(mediaKindsFor([lib('movies', true, [['wittyvideo', true]])]), ['video'])
})

test('before the server has said what it has, only video is claimed', () => {
  assert.deepEqual(mediaKindsFor(null), ['video'])
  assert.deepEqual(deviceFeatures(['video']), FEATURES)
})

test('queue and navigate are the device\'s own, whatever it plays', () => {
  assert.deepEqual(deviceFeatures([]), ['queue', 'navigate'])
  assert.deepEqual(deviceFeatures(['game']), ['queue', 'navigate'])
  assert.deepEqual(deviceFeatures(['radio']), ['volume', 'queue', 'navigate'])
  assert.deepEqual(deviceFeatures(['book']), ['queue', 'navigate'])
})

test('each player accepts what it can really do', () => {
  assert.ok(commandsOf('video').length >= 12)
  for (const c of ['pause', 'resume', 'stop', 'seek', 'next', 'previous', 'volume', 'mute', 'audio', 'subtitle', 'rate', 'navigate']) assert.ok(accepts('video', c), c)
  // A station is live.
  assert.equal(accepts('radio', 'seek'), false)
  assert.equal(accepts('radio', 'rate'), false)
  assert.ok(accepts('radio', 'next'))
  // Music has no sound tracks or subtitles to choose.
  assert.equal(accepts('audio', 'audio'), false)
  assert.equal(accepts('audio', 'subtitle'), false)
  assert.ok(accepts('audio', 'rate'))
  // A CD plays at one speed.
  assert.equal(accepts('cd', 'rate'), false)
  assert.ok(accepts('cd', 'seek'))
  // A disc: chapters, tracks, and its menu keys.
  for (const c of ['seek', 'next', 'previous', 'audio', 'subtitle', 'navigate']) assert.ok(accepts('disc', c), c)
  // Live TV has no speed.
  assert.equal(accepts('tv', 'rate'), false)
  assert.ok(accepts('tv', 'next'))
  // A book turns pages and nothing else.
  assert.deepEqual(commandsOf('book'), ['stop', 'next', 'previous', 'navigate'])
  assert.equal(accepts('photo', 'seek'), false)
  assert.ok(accepts('photo', 'pause'))
  assert.deepEqual(commandsOf('game'), ['stop'])
  // Every player can be stopped; none is asked to `play` (the device opens things itself).
  for (const p of ['video', 'disc', 'cd', 'audio', 'radio', 'tv', 'book', 'photo', 'game']) { assert.ok(accepts(p, 'stop'), p); assert.equal(accepts(p, 'play'), false) }
  assert.equal(accepts('video', 'dance'), false)
})

test('features follow from the commands', () => {
  assert.deepEqual(featuresOf('game'), [])
  assert.deepEqual(featuresOf('book'), ['queue', 'navigate'])
  assert.deepEqual(featuresOf('radio'), ['queue', 'volume', 'navigate'])
  assert.deepEqual(featuresOf('video').sort(), [...FEATURES].sort())
})

test('a refusal says what can\'t be done, for a person', () => {
  assert.equal(refusal('radio', 'seek'), 'A radio station can’t be moved through.')
  assert.equal(refusal('book', 'pause'), 'A book can’t be paused.')
  assert.equal(refusal('game', 'navigate'), 'A game is being played there: its own controller steers it.')
  assert.equal(refusal('cd', 'rate'), 'A CD can’t change speed.')
  assert.equal(refusal('video', 'dance'), 'This device can’t do that.')
  for (const p of ['video', 'disc', 'cd', 'audio', 'radio', 'tv', 'book', 'photo', 'game']) for (const c of ['pause', 'seek', 'rate', 'audio', 'subtitle', 'volume', 'next', 'navigate']) assert.ok(refusal(p, c).length <= 200)
})

test('play opens the right player for the kind', () => {
  assert.equal(routeFor({ id: 7, kind: 'video' }), '/play/7')
  assert.equal(routeFor({ id: 7, kind: 'video' }, { positionSec: 754.5, audio: '2', subtitle: 'off', remote: 'c1' }), '/play/7?start=754.5&audio=2&subtitle=off&remote=c1')
  assert.equal(routeFor({ id: 7, kind: 'video' }, { positionSec: 0 }), '/play/7?start=0')
  // A disc opens on its menu, unless it is told where in the film to start.
  assert.equal(routeFor({ id: 9, kind: 'dvd-iso' }), '/play/9')
  assert.equal(routeFor({ id: 9, kind: 'bluray-folder' }, { positionSec: 600 }), '/play/9?start=600&mode=title')
  assert.equal(routeFor({ id: 4, kind: 'cd-image' }), '/cd/4?play=1')
  assert.equal(routeFor({ id: 5, kind: 'book', reader: 'epub' }), '/read/5')
  assert.equal(routeFor({ id: 5, kind: 'book' }), null) // a format with no reader here
  assert.equal(routeFor({ id: 6, kind: 'game' }), '/game/6')
  assert.equal(routeFor({ id: 8, kind: 'photo', libraryId: 3 }), '/library/3?view=8')
  // Music and stations have no page: the audio player takes them.
  assert.equal(routeFor({ id: 2, kind: 'audio' }), null)
  assert.equal(routeFor({ id: 2, kind: 'radio' }), null)
})

test('one queue is one player\'s', () => {
  assert.equal(queueFamily('video'), 'video')
  assert.equal(queueFamily('disc'), 'video')
  assert.equal(queueFamily('audio'), 'audio')
  assert.equal(queueFamily('radio'), 'radio')
  assert.equal(queueFamily('book'), 'single')
  assert.equal(checkQueue(['video']), '')
  assert.equal(checkQueue(['video', 'video', 'dvd-iso']), '')
  assert.equal(checkQueue(['audio', 'audio']), '')
  assert.equal(checkQueue(['radio', 'radio']), '')
  assert.match(checkQueue(['video', 'audio']), /all videos or all music/)
  assert.match(checkQueue(['audio', 'radio']), /all videos or all music/)
  assert.match(checkQueue(['book', 'book']), /by itself/)
  assert.equal(checkQueue(['book']), '')
  assert.match(checkQueue(['video', 'hologram']), /can’t play that kind/)
})

test('the remote\'s keys are the keys a TV remote sends', () => {
  for (const k of NAV_KEYS) assert.ok(isNavKey(k), k)
  assert.equal(isNavKey('eject'), false)
  assert.equal(isNavKey(undefined), false)
  assert.equal(keyboardKey('up'), 'ArrowUp')
  assert.equal(keyboardKey('select'), 'Enter')
  assert.equal(keyboardKey('back'), 'Escape')
  assert.equal(keyboardKey('menu'), 'ContextMenu')
  assert.equal(keyboardKey('home'), null)
  assert.equal(keyboardKey('playpause'), null)
})

test('where the browser is, in a word', () => {
  assert.equal(screenFor('/'), 'home')
  assert.equal(screenFor('/play/4'), 'player')
  assert.equal(screenFor('/tv/watch/abc'), 'tv')
  assert.equal(screenFor('/tv'), 'guide')
  assert.equal(screenFor('/read/9'), 'reader')
  assert.equal(screenFor('/library/2'), 'browse')
  assert.equal(screenFor('/settings/sharing'), 'settings')
})

const device = (over = {}) => ({
  online: true, capabilities: { mediaKinds: ['video', 'audio', 'radio'], features: FEATURES }, itemKind: 'video',
  state: { status: 'playing', durationSec: 5400, volume: 0.6, audioTracks: [{ id: '1' }, { id: '2' }], subtitleTracks: [{ id: '3' }], queue: [1, 2, 3] }, ...over,
})

test('"Play here" only where the device said it plays that kind', () => {
  assert.ok(canPlayOn(device(), 'video'))
  assert.equal(canPlayOn(device(), 'book'), false)
  assert.equal(canPlayOn(device({ online: false }), 'video'), false)
})

test('a remote shows what the device claimed, narrowed to what it is playing', () => {
  assert.deepEqual(remoteControls(device()), { transport: true, seek: true, steps: true, volume: true, rate: true, audio: true, subtitles: true, queue: true, dpad: true, place: 'time' })
  // A station: no scrubber, no speed.
  const radio = remoteControls(device({ itemKind: 'radio', state: { status: 'playing', durationSec: 0, volume: 1 } }))
  assert.equal(radio.seek, false); assert.equal(radio.rate, false); assert.equal(radio.place, 'none'); assert.ok(radio.transport); assert.ok(radio.steps)
  // A book: page turns and the D-pad; its place is a share, not a time.
  const book = remoteControls(device({ itemKind: 'book', state: { status: 'paused', durationSec: 300 } }))
  assert.deepEqual(book, { transport: false, seek: false, steps: true, volume: false, rate: false, audio: false, subtitles: false, queue: false, dpad: true, place: 'part' })
  // A game: nothing but stop (which every remote has) and no keys offered that it would refuse.
  const game = remoteControls(device({ itemKind: 'game', state: { status: 'playing' } }))
  assert.equal(game.transport, false); assert.equal(game.steps, false); assert.equal(game.seek, false)
  // One sound track is nothing to choose between.
  assert.equal(remoteControls(device({ state: { status: 'playing', durationSec: 60, audioTracks: [{ id: '1' }] } })).audio, false)
  // A device that claims less is offered less.
  const small = remoteControls(device({ capabilities: { mediaKinds: ['video'], features: ['seek', 'volume'] } }))
  assert.equal(small.dpad, false); assert.equal(small.rate, false); assert.equal(small.steps, false); assert.equal(small.queue, false); assert.ok(small.seek)
  // Nothing playing: the D-pad still drives its interface, and its volume if it says one.
  const idle = remoteControls(device({ itemKind: undefined, state: { status: 'idle', volume: 0.5 } }))
  assert.deepEqual(idle, { transport: false, seek: false, steps: false, volume: true, rate: false, audio: false, subtitles: false, queue: false, dpad: true, place: 'none' })
  // Offline: nothing.
  const off = remoteControls(device({ online: false }))
  assert.equal(Object.values(off).filter((v) => v === true).length, 0)
})

test('something the controller may not see shows nothing of itself', () => {
  const hidden = remoteControls(device({ hidden: true, itemKind: undefined, state: { status: 'playing', volume: 0.4 } }))
  assert.equal(hidden.place, 'none'); assert.equal(hidden.seek, false); assert.equal(hidden.queue, false); assert.equal(hidden.audio, false)
  assert.ok(hidden.transport); assert.ok(hidden.volume)
})

test('live TV on a channel that is not a library item still gets its keys', () => {
  const tv = device({ itemKind: undefined, state: { status: 'idle', screen: 'tv', volume: 1 } })
  assert.ok(watchingTv(tv))
  const c = remoteControls(tv)
  assert.ok(c.transport); assert.ok(c.steps); assert.ok(c.volume); assert.equal(c.seek, false); assert.equal(c.place, 'none')
  assert.equal(watchingTv(device({ state: { status: 'idle', screen: 'home' } })), false)
})

test('errors are worded by code, with the server\'s reason where it has one', () => {
  assert.equal(errorText('offline', 'That device isn\'t connected.', 'Den TV'), 'Den TV isn’t connected right now.')
  assert.equal(errorText('refused', 'A radio station can’t be moved through.', 'Den TV'), 'A radio station can’t be moved through.')
  assert.equal(errorText('refused', '', 'Den TV'), 'Den TV wouldn’t do that.')
  assert.equal(errorText('unsupported', 'That device can\'t play that kind of thing.', 'Den TV'), 'That device can\'t play that kind of thing.')
  assert.match(errorText('timeout', 'x', 'Den TV'), /Den TV didn’t answer/)
  assert.match(errorText('busy', 'x', 'Den TV'), /too much to do/)
  assert.match(errorText('rate', 'x', 'Den TV'), /too fast/)
  assert.match(errorText('not-allowed-there', '', 'Kids TV'), /Kids TV is signed in as isn’t allowed/)
  assert.equal(errorText(undefined, 'Can\'t reach your server.', 'Den TV'), 'Can\'t reach your server.')
  assert.equal(errorText(undefined, '', ''), 'That didn’t work.')
  assert.match(errorText('offline', '', ''), /^That device/)
})

test('next and previous are named for what they step through', () => {
  assert.deepEqual(stepWords(device()), { next: 'Next', previous: 'Previous' })
  assert.deepEqual(stepWords(device({ itemKind: 'dvd-iso' })), { next: 'Next chapter', previous: 'Previous chapter' })
  assert.deepEqual(stepWords(device({ itemKind: 'cd-image' })), { next: 'Next track', previous: 'Previous track' })
  assert.deepEqual(stepWords(device({ itemKind: 'book' })), { next: 'Next page', previous: 'Previous page' })
  assert.deepEqual(stepWords(device({ itemKind: 'photo' })), { next: 'Next picture', previous: 'Previous picture' })
  assert.deepEqual(stepWords(device({ itemKind: 'radio' })), { next: 'Next station', previous: 'Previous station' })
  assert.deepEqual(stepWords(device({ itemKind: undefined, state: { status: 'idle', screen: 'tv' } })), { next: 'Channel up', previous: 'Channel down' })
  // Something hidden gives nothing away, not even what kind of thing it is.
  assert.deepEqual(stepWords(device({ hidden: true, itemKind: 'book' })), { next: 'Next', previous: 'Previous' })
})

test('what a device is doing, in a few words', () => {
  assert.equal(statusWords(device()), 'Playing')
  assert.equal(statusWords(device({ state: { status: 'paused' } })), 'Paused')
  assert.equal(statusWords(device({ state: { status: 'buffering' } })), 'Loading')
  assert.equal(statusWords(device({ state: { status: 'idle' } })), 'Nothing playing')
  assert.equal(statusWords(device({ state: { status: 'idle', screen: 'tv' } })), 'Live TV')
  assert.equal(statusWords(device({ online: false })), 'Not connected')
  assert.equal(statusWords(device({ hidden: true })), 'Playing something')
  assert.equal(statusWords(device({ hidden: true, state: { status: 'paused' } })), 'Paused')
})
