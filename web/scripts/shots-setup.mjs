// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Takes the pictures in the first-time setup guide, on a test server with a fresh data folder. It
// walks setup as a new owner would: no native window opens, and Choose folder is never pressed.
// Each picture is named after the page it shows (setup-<page>.png, the ids in src/pages/setup/plan.ts).
//
// Started with no -data, a desktop server asks where to keep its data; give it its own APPDATA,
// LOCALAPPDATA and WMS_DOCUMENTS_DIR so nothing outside the test folder is touched.
//
// Before the first library is added it turns off saving details beside the media and the free online
// sources, so nothing is written next to the media and only what's beside it shows.
//
//   WMS_SERVER=http://demo-media.localhost:19701 WMS_MEDIA=/srv/media node scripts/shots-setup.mjs out-folder
import { open, wait } from './shots-lib.mjs'

const server = process.env.WMS_SERVER
const media = process.env.WMS_MEDIA || '/srv/media'
const [out = 'shots'] = process.argv.slice(2)
const s = await open(server, out)
const { page, press, shot, card, call, until, typeIn, maskAddresses } = s
const CARD = 'main.oobe-card'
const here = () => page.eval(`location.pathname.split('/').pop()`)
const atStep = (id) => until(`location.pathname === '/setup/${id}' && document.querySelector('main h1, main h2')`, 40000, `step ${id}`)
const onward = () => press(/^\s*Continue\s*$|^\s*Continue anyway\s*$/, 'main')
const text = () => page.eval(`document.querySelector('main')?.innerText || ''`)
// WMS_SHOW_PATHS="<this computer's folder>=><the folder to show>;..." puts example folders in the pictures in place of
// the test computer's own, as maskAddresses does for addresses.
const PATHS = (process.env.WMS_SHOW_PATHS || '').split(';').filter(Boolean).map((p) => p.split('=>'))
const maskPaths = () => PATHS.length && page.eval(`(() => {
  const swaps = ${JSON.stringify(PATHS)}
  const w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
  for (let n = w.nextNode(); n; n = w.nextNode()) for (const [from, to] of swaps) if (n.nodeValue.includes(from)) n.nodeValue = n.nodeValue.split(from).join(to)
})()`)
const MODULES = [/WittyPlay/, /WittyDVD/, /WittyBluray/, /WittyCD/, /WittyTuner/, /WittyPhotos/, /WittyEbook/, /WittyPDF/, /WittyComics/, /WittyVisualizer/, /WittyStats/]

try {
  await wait(1200)
  await until(`location.pathname.startsWith('/setup')`)
  await atStep('language'); await wait(600)
  await card('setup-language', CARD)
  await onward()
  await atStep('welcome'); await wait(600)
  await card('setup-welcome', CARD)
  await press('Set up a new server')
  await atStep('account'); await wait(500)
  await typeIn('main input[autocomplete=username], main input[type=text], main input:not([type])', 'owner')
  const pw = await page.eval(`[...document.querySelectorAll('main input[type=password]')].length`)
  for (let i = 0; i < pw; i++) {
    await page.eval(`document.querySelectorAll('main input[type=password]')[${i}].focus()`)
    await page.send('Input.insertText', { text: 'demo-only-password-77' })
  }
  await press(/I understand/, 'main')
  await wait(300)
  await card('setup-account', CARD)
  await press(/Create account/)
  await until(`!/\\/setup\\/(account|welcome|language)$/.test(location.pathname)`, 40000, 'leaving the account page')
  // Signed in now: nothing goes beside the media, and only what's beside it is used.
  console.log('settings', JSON.stringify(await call('PUT', '/settings', { saveMetadata: '0', saveWittyinfo: '0', tvmaze: '0', wikidata: '0', musicbrainz: '0' })))
  const seen = new Set()
  for (let i = 0; i < 40; i++) {
    await wait(1200)
    const path = await page.eval(`location.pathname`)
    if (!path.startsWith('/setup/')) break
    const id = await here()
    if (id === 'done') break
    if (seen.has(id)) { console.log('back on', id, (await text()).slice(0, 300)); await wait(1500) }
    seen.add(id)
    await wait(600)
    console.log('step', id, (await text()).replace(/\s+/g, ' ').slice(0, 400))
    if (id === 'storage') {
      await maskAddresses()
      await maskPaths()
      await card('setup-storage', CARD)
      // Documents is chosen to begin with, and it's in this test's own folder.
      await press(/Keep it here|Move it|Use this folder|^\s*Continue\s*$/, 'main')
      await until(`location.pathname !== '/setup/storage'`, 120000, 'the data folder').catch((e) => console.log(e.message))
      continue
    }
    if (id === 'name') { await maskAddresses(); await card('setup-name', CARD); await onward(); continue }
    if (id === 'modules') {
      await wait(1200)
      await card('setup-modules', CARD)
      for (const m of MODULES) { console.log('tick', m, await press(m, 'main')); await wait(120) }
      await wait(300)
      await card('setup-modules-chosen', CARD)
      await press(/Download and install/)
      await wait(1000)
      await card('setup-modules-installing', CARD)
      await until(`/ready|didn.t install/i.test(document.querySelector('main h1, main h2')?.textContent || '')`, 400000, 'modules to install')
      await wait(400)
      await card('setup-modules-ready', CARD)
      await press(/^\s*Continue|^\s*Next/, 'main')
      continue
    }
    if (id === 'media') {
      await card('setup-media', CARD)
      await press(/^\s*Movies|Movies/, 'main'); await wait(500)
      await typeIn('main input[aria-label="Folder path"], main input[placeholder*="/"], main input[placeholder*="\\\\"], main input[type=text]:not([placeholder="Movies"])', media + '/Movies')
      await wait(200)
      await card('setup-media-movies', CARD)
      await press(/Add Movies/, 'main'); await wait(2200)
      await card('setup-media-added', CARD)
      await onward()
      continue
    }
    if (id === 'logos') { await card('setup-logos', CARD); await press(/No thanks/, 'main'); continue }
    if (id === 'listings') { await card('setup-listings', CARD); await press(/Leave it off/, 'main'); continue }
    if (id === 'tv') {
      await card('setup-tv', CARD)
      // Live TV: channels come later, from Settings, so this is passed by.
      if (!(await onward())) await press(/Skip for now|Not now|Later/, 'main')
      continue
    }
    if (id === 'people') { await card('setup-people', CARD); await onward(); continue }
    if (id === 'look') {
      // The website shows this page as a whole window, 1440 wide.
      await s.size(1440, 900); await wait(1200)
      await shot('setup-look-1440')
      await s.size(1920, 1080); await wait(800)
    }
    await card(`setup-${id}`, CARD)
    if (!(await onward())) {
      console.log('no Continue on', id, '- skipping')
      await press(/Skip for now/)
    }
  }
  await until(`location.pathname === '/setup/done'`, 20000, 'the last page')
  await wait(1500)
  await maskAddresses()
  await card('setup-done', CARD)
  console.log(await text())
  await press(/Start watching/)
  await until(`!location.pathname.startsWith('/setup')`, 15000, 'leaving setup')
  await wait(2500)
  await shot('setup-99-home')
  console.log('settings now', JSON.stringify(await call('GET', '/settings')).slice(0, 400))
} catch (e) {
  console.log('STOPPED', e.message, await page.eval('location.pathname'))
  await shot('setup-stopped').catch(() => {})
  process.exitCode = 1
} finally {
  page.close()
  await s.browser.close()
}
