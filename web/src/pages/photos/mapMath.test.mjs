import test from 'node:test'
import assert from 'node:assert/strict'
import { clampView, fitView, groupZoom, latOf, lonOf, mercX, mercY, minZoom, placeName, scaleOf, tileUrl, toScreen, viewBox, worldCopies, zoomAt } from './mapMath.ts'

const close = (a, b, d = 1e-6) => assert.ok(Math.abs(a - b) < d, `${a} is not ${b}`)
const phone = { w: 375, h: 500 }, tv = { w: 1920, h: 1080 }

test('places go to the map and back', () => {
  for (const [lat, lon] of [[0, 0], [51.5, -0.12], [-33.86, 151.2], [64.1, -21.9], [-54.8, -68.3]]) {
    close(latOf(mercY(lat)), lat, 1e-9)
    close(lonOf(mercX(lon)), lon, 1e-9)
  }
  close(mercY(0), 0.5)
  assert.ok(mercY(89.9) >= 0 && mercY(-90) <= 1, 'the poles stay on the map')
})

test('zoomed right out, the world fits the shorter side', () => {
  close(scaleOf(minZoom(phone)), 375)
  close(scaleOf(minZoom(tv)), 1080)
  assert.equal(minZoom({ w: 200, h: 100 }), 0)
  const v = clampView({ x: 0.5, y: 0.1, z: -3 }, tv)
  assert.equal(v.y, 0.5, 'a world shorter than the map sits in the middle')
})

test('the map goes round sideways but not off the top', () => {
  const v = clampView({ x: 1.25, y: -1, z: 5 }, tv)
  close(v.x, 0.25)
  close(v.y, tv.h / 2 / scaleOf(5))
  close(clampView({ x: -0.25, y: 0.5, z: 5 }, tv).x, 0.75)
})

test('zooming keeps the place under the pointer', () => {
  const v = { x: 0.52, y: 0.33, z: 6 }
  const px = 300, py = 200
  const before = { x: v.x + (px - tv.w / 2) / scaleOf(v.z), y: v.y + (py - tv.h / 2) / scaleOf(v.z) }
  const w = zoomAt(v, tv, px, py, 1.5)
  const p = toScreen(w, tv, before.x, before.y)
  close(p.left, px, 1e-6)
  close(p.top, py, 1e-6)
})

test('fitting shows the box, the short way round the world', () => {
  const v = fitView({ south: 40, west: -10, north: 60, east: 30 }, tv)
  const sw = toScreen(v, tv, mercX(-10), mercY(40)), ne = toScreen(v, tv, mercX(30), mercY(60))
  assert.ok(sw.left >= 0 && ne.left <= tv.w && ne.top >= 0 && sw.top <= tv.h, 'all in view')
  // Fiji (178°E) to Samoa (172°W): across the date line, not the long way.
  const p = fitView({ south: -20, west: 178, north: -13, east: -172 }, tv)
  assert.ok(p.z > 4, `zoomed in on the Pacific, not the world: ${p.z}`)
  close(lonOf(p.x) > 180 ? lonOf(p.x) - 360 : lonOf(p.x), -177, 0.5)
  // One picture: as close as allowed, not infinitely close.
  assert.equal(fitView({ south: 1, west: 1, north: 1, east: 1 }, tv, 14).z, 14)
})

test('the server is asked for the part in view, wrapped at the date line', () => {
  const b = viewBox({ x: mercX(180), y: mercY(-17), z: 6 }, tv)
  assert.ok(b.west > b.east, `across the date line: ${JSON.stringify(b)}`)
  assert.ok(b.west >= -180 && b.east <= 180 && b.south < -17 && b.north > -17)
  const all = viewBox({ x: 0.5, y: 0.5, z: minZoom(tv) }, tv)
  assert.deepEqual([all.west, all.east, all.south, all.north], [-180, 180, -90, 90])
  // Small moves ask the same thing, so the answer is reused.
  const v = { x: 0.51, y: 0.34, z: 7.3 }
  assert.deepEqual(viewBox(v, tv), viewBox({ ...v, x: v.x + 3 / scaleOf(v.z) }, tv))
  assert.equal(groupZoom(7.99), 7)
  assert.equal(groupZoom(8), 8)
})

test('each copy of the world in view is drawn', () => {
  assert.deepEqual(worldCopies({ x: 0.5, y: 0.5, z: minZoom(tv) }, tv), [-1, 0, 1])
  assert.deepEqual(worldCopies({ x: 0.999, y: 0.5, z: 8 }, tv), [0, 1])
  assert.deepEqual(worldCopies({ x: 0.5, y: 0.5, z: 8 }, tv), [0])
})

test('online map addresses', () => {
  assert.equal(tileUrl('https://{s}.tiles.example/{z}/{x}/{y}{r}.png', 3, 4, 5), 'https://a.tiles.example/3/4/5.png')
})

test('places are named by their coordinates', () => {
  assert.equal(placeName(48.8566, 2.3522), '48.86° N, 2.35° E')
  assert.equal(placeName(-33.8688, 151.2093), '33.87° S, 151.2° E')
  assert.equal(placeName(21.3, -157.8 - 360), '21.30° N, 157.8° W')
})
