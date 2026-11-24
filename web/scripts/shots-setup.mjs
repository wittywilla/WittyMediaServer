// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Takes the pictures in the first-time setup guide, on a test server with a fresh data folder. It
// walks setup as a new owner would: no native window opens, and Choose folder is never pressed.
// MODE=advanced walks advanced setup instead, for the pictures of the pages only it has.
//
//   WMS_SERVER=http://demo-media.localhost:19701 WMS_MEDIA=/srv/media node scripts/shots-setup.mjs out-folder
//   MODE=advanced WMS_SERVER=http://demo-media2.localhost:19702 node scripts/shots-setup.mjs out-folder
import { open, wait } from './shots-lib.mjs'

const server = process.env.WMS_SERVER
const media = process.env.WMS_MEDIA || '/srv/media'
const advanced = process.env.MODE === 'advanced'
const [out = 'shots'] = process.argv.slice(2)
const s = await open(server, out, { width: 1440, height: 1000 })
const { page, press, shot, card, until, typeIn, maskAddresses } = s
const CARD = 'main.oobe-card'
const DIALOG = '.ui-dialog'
const atStep = (id) => until(`location.pathname === '/setup/${id}' && document.querySelector('main h1')`, 40000, `step ${id}`)
const onward = () => press(/^\s*Continue\s*$/, 'main')
// WMS_SHOW_PATHS="<this computer's folder>=><the folder to show>;..." puts example folders in the pictures in place of
// the test computer's own, as maskAddresses does for addresses.
const PATHS = (process.env.WMS_SHOW_PATHS || '').split(';').filter(Boolean).map((p) => p.split('=>'))
const maskPaths = () => PATHS.length && page.eval(`(() => {
  const swaps = ${JSON.stringify(PATHS)}
  const w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
  for (let n = w.nextNode(); n; n = w.nextNode()) for (const [from, to] of swaps) if (n.nodeValue.includes(from)) n.nodeValue = n.nodeValue.split(from).join(to)
})()`)
// A desktop server started without -data asks where to keep its data, right after the account.
const storage = async () => {
  await until(`location.pathname !== '/setup/account'`, 40000, 'leaving the account page')
  await wait(1500)
  if ((await page.eval('location.pathname')) !== '/setup/storage') return
  await maskAddresses(); await maskPaths()
  await card('setup-03-data-folder', CARD)
  await press(/Keep it here|^\s*Continue\s*$/, 'main')
  await until(`location.pathname !== '/setup/storage'`, 120000, 'the data folder')
}
// A step's Customize dialog, with one of its parts opened.
const customize = async (name, part) => {
  await press(/^\s*Customize\s*$/, 'main')
  await until(`!!document.querySelector('${DIALOG}')`, 5000, 'the Customize dialog')
  if (part) await page.eval(`(() => { const d = [...document.querySelectorAll('${DIALOG} details.su-option')].find((x) => x.querySelector('summary b')?.textContent === ${JSON.stringify(part)}); if (d) d.open = true })()`)
  await wait(1200)
  await card(name, DIALOG)
  await press(/^\s*Done\s*$/, DIALOG)
  await wait(400)
}
const account = async () => {
  await typeIn('main input[autocomplete=username]', 'owner')
  for (let i = 0; i < 2; i++) {
    await page.eval(`document.querySelectorAll('main input[type=password]')[${i}].focus()`)
    await page.send('Input.insertText', { text: 'demo-only-password-77' })
  }
  if (!advanced) await typeIn('main input[placeholder="WittyMediaServer"]', 'Living Room')
  await press(/I understand/, 'main')
  await wait(300)
}

try {
  await wait(1200)
  await atStep('welcome')
  if (advanced) {
    await press('Advanced setup')
    await atStep('locale'); await wait(400)
    await card('setup-20-advanced-language', CARD)
    await onward()
    await atStep('account'); await wait(500)
    await account()
    await card('setup-21-advanced-account', CARD)
    await press(/Create account/)
    await atStep('name'); await wait(1200)
    await card('setup-22-advanced-name', CARD)
    await onward()
    await atStep('people'); await wait(800)
    await card('setup-23-advanced-people', CARD)
    await onward()
    await atStep('modules'); await wait(1800)
    await press(/Install \d+ module/, 'main')
    await until(`/ready|didn.t install/i.test(document.querySelector('main h1')?.textContent || '')`, 300000, 'modules to install')
    await onward()
    await atStep('media'); await wait(800)
    await card('setup-24-advanced-media', CARD)
    await press(/^\s*Skip for now\s*$/, 'main')
    for (const [id, name] of [['details', 'setup-25-advanced-details'], ['logos', 'setup-26-advanced-logos'], ['music', 'setup-27-advanced-music'], ['karaoke', 'setup-28-advanced-karaoke'], ['listings', 'setup-29-advanced-listings']]) {
      await atStep(id); await wait(1500)
      await card(name, CARD)
      await onward()
    }
    await atStep('look'); await wait(800)
    await onward()
    await atStep('colours'); await wait(1200)
    await card('setup-30-advanced-colours', CARD)
    await onward()
    await atStep('network'); await wait(1200)
    await maskAddresses()
    await card('setup-31-advanced-network', CARD)
    await onward()
    await atStep('remote'); await wait(1500)
    await maskAddresses()
    await card('setup-32-advanced-remote', CARD)
    await onward()
    await atStep('done')
  } else {
    await shot('setup-01-welcome')
    await customize('setup-02-welcome-customize')
    await press('Get started')
    await atStep('account'); await wait(500)
    await account()
    await card('setup-03-account', CARD)
    await press(/Create account/)
    await storage()
    await atStep('modules'); await wait(1800)
    for (const m of [/WittyPlay/, /WittyComics/, /WittyPhotos/]) { console.log('tick', m, await press(m, 'main')); await wait(120) }
    await card('setup-04-modules', CARD)
    await press(/Install \d+ module/)
    await wait(1000)
    await card('setup-05-modules-installing', CARD, 16, { allow: ['loading'] }) // the modules being installed, on purpose
    await until(`/ready|didn.t install/i.test(document.querySelector('main h1')?.textContent || '')`, 300000, 'modules to install')
    await onward()
    await atStep('media'); await wait(800)
    await card('setup-06-media', CARD)
    await customize('setup-07-media-customize', 'Posters and details')
    // The demo media is shared: nothing is written beside it.
    // Online sources off too: they attach posters that aren't openly licensed.
    await s.call('PUT', '/settings', { saveMetadata: '0', saveWittyinfo: '0', tvmaze: '0', wikidata: '0', musicbrainz: '0' })
    await press(/^\s*Movies/, 'main'); await wait(500)
    await typeIn('main input[aria-label="Folder path"]', media + '/Movies')
    await wait(200)
    await card('setup-08-media-movies', CARD)
    await press(/Add Movies/, 'main'); await wait(1800)
    await card('setup-09-media-added', CARD)
    await onward()
    await atStep('games'); await wait(2500)
    await card('setup-10-games', CARD)
    await onward()
    await atStep('look'); await wait(1200)
    await card('setup-11-look', CARD)
    await customize('setup-12-look-customize', 'Colours')
    await onward()
    await atStep('done'); await wait(1500)
    await maskAddresses()
    await card('setup-13-done', CARD)
    await customize('setup-14-done-customize')
  }
  await press(/Start watching/)
  await until(`!location.pathname.startsWith('/setup')`, 15000, 'leaving setup')
  await wait(2500)
  console.log('saveMetadata', (await s.call('GET', '/settings'))?.saveMetadata)
} catch (e) {
  console.log('STOPPED', e.message, await page.eval('location.pathname'))
  await shot('setup-stopped').catch(() => {})
  process.exitCode = 1
} finally {
  page.close()
  await s.browser.close()
}
