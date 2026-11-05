import test from 'node:test'
import assert from 'node:assert/strict'

// A browser's storage, for the functions that keep viewing preferences.
const store = new Map()
globalThis.localStorage = {
  get length() { return store.size },
  key: (i) => [...store.keys()][i] ?? null,
  getItem: (k) => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: (k) => store.delete(k),
}
const { loadView, saveView, saveScope, setGlobalView, globalView } = await import('./viewPrefs.ts')
const get = (k) => JSON.parse(localStorage.getItem(k) || 'null')

test('3D saved for everything by an earlier version is cleared', () => {
  store.clear()
  // What the owner had: 3D on for everything, and copied into a disc of its own.
  localStorage.setItem('wms.view', JSON.stringify({ seekBar: false, stereo: 'sbs', stereoOut: 'anaglyph' }))
  localStorage.setItem('wms.view.item.7', JSON.stringify({ picture: 'fill', stereo: 'sbs' }))
  localStorage.setItem('wms.disc.7', JSON.stringify({ audioLanguage: 'en' }))
  const v = loadView(9)
  assert.equal(v.prefs.stereo, 'off')
  assert.equal(v.prefs.seekBar, false, 'the rest of what was saved stays')
  assert.deepEqual(get('wms.view'), { seekBar: false })
  assert.deepEqual(get('wms.view.item.7'), { picture: 'fill' })
  assert.deepEqual(get('wms.disc.7'), { audioLanguage: 'en' }, 'other keys are left alone')
  assert.equal(loadView(7).prefs.stereo, 'off')
})

test('3D is kept for one disc, whatever Apply to says', () => {
  store.clear()
  let v = loadView(1)
  assert.equal(v.scope, 'global')
  const on = saveView(1, v.prefs, { stereo: 'sbs' }, 'global')
  assert.equal(on.stereo, 'sbs')
  assert.equal(get('wms.view'), null, 'nothing shared was written')
  assert.equal(loadView(1).prefs.stereo, 'sbs')
  assert.equal(loadView(2).prefs.stereo, 'off', 'another disc starts in 2D')
  // Shared changes still follow Apply to, and never carry 3D with them.
  saveView(1, on, { picture: 'fill' }, 'global')
  assert.deepEqual(get('wms.view'), { picture: 'fill' })
  saveScope(1, loadView(1).prefs, 'item')
  assert.equal(get('wms.view.item.1').stereo, undefined)
  saveScope(1, loadView(1).prefs, 'global')
  assert.equal(get('wms.view').stereo, undefined)
  assert.equal(loadView(2).prefs.stereo, 'off')
  // Appearance's settings for every video can't carry 3D either.
  setGlobalView({ stereo: 'sbs', seekBar: false })
  assert.equal(get('wms.view').stereo, undefined)
  assert.equal(get('wms.view').seekBar, false)
  assert.equal(globalView().stereo, 'off')
  // Off again: nothing kept.
  saveView(1, loadView(1).prefs, { stereo: 'off' }, 'item')
  assert.equal(localStorage.getItem('wms.view.3d.1'), null)
})
