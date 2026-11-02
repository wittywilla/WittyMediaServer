// Takes the pictures in the first-time setup guide, on a test server with a fresh data folder. It
// walks setup as a new owner would: no native window opens, and Choose folder is never pressed.
//
//   WMS_SERVER=http://demo-media.localhost:19701 WMS_MEDIA=/srv/media node scripts/shots-setup.mjs out-folder
import { open, wait } from './shots-lib.mjs'

const server = process.env.WMS_SERVER
const media = process.env.WMS_MEDIA || '/srv/media'
const [out = 'shots'] = process.argv.slice(2)
const s = await open(server, out)
const { page, press, shot, card, until, typeIn, maskAddresses } = s
const CARD = 'main.oobe-card'
const atStep = (id) => until(`location.pathname === '/setup/${id}' && document.querySelector('main h1')`, 40000, `step ${id}`)
const onward = () => press(/^\s*Continue/, 'main')

try {
  await wait(1200)
  await until(`location.pathname.startsWith('/setup')`)
  await shot('setup-01-welcome')
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
  await card('setup-02-account', CARD)
  await press(/Create account/)
  await atStep('modules'); await wait(1800)
  await card('setup-03-modules', CARD)
  for (const m of [/WittyPlay/, /WittyDVD/, /WittyBluray/, /WittyCD/, /WittyPhotos/, /WittyEbook/, /WittyPDF/, /WittyComics/, /WittyVisualizer/, /WittyStats/]) {
    console.log('tick', m, await press(m, 'main')); await wait(120)
  }
  await wait(300)
  await card('setup-04-modules-chosen', CARD)
  await press(/Download and install/)
  await wait(1000)
  await card('setup-05-modules-installing', CARD)
  await until(`/ready|didn.t install/i.test(document.querySelector('h1')?.textContent || '')`, 300000, 'modules to install')
  await wait(400)
  await card('setup-06-modules-ready', CARD)
  await press(/Continue|Next/, 'main')
  await atStep('media'); await wait(800)
  await card('setup-07-media', CARD)
  await press(/^\s*Movies|Movies/, 'main'); await wait(500)
  await typeIn('main input[aria-label="Folder path"], main input[placeholder*="/"], main input[type=text]:not([placeholder="Movies"])', media + '/Movies')
  await wait(200)
  await card('setup-08-media-movies', CARD)
  await press(/Add Movies/, 'main'); await wait(1800)
  await card('setup-09-media-added', CARD)
  await onward()
  const seen = new Set()
  let n = 10
  for (let i = 0; i < 20; i++) {
    await wait(900)
    const id = (await page.eval(`location.pathname`)).split('/').pop()
    if (id === 'done' || !(await page.eval(`location.pathname`)).startsWith('/setup/')) break
    if (id === 'media') { await onward(); continue }
    if (seen.has(id)) { console.log('stuck on', id); await wait(1500) }
    seen.add(id)
    await wait(500)
    await card(`setup-${n++}-${id}`, CARD)
    console.log('step', id, (await page.eval(`document.querySelector('main').innerText`)).slice(0, 900))
    if (id === 'details') {
      console.log('saved', await press(/Save posters and details/, 'main')); await wait(250)
      await card(`setup-${n++}-${id}-chosen`, CARD)
    }
    if (id === 'logos') { await press(/No thanks/, 'main'); await wait(500); continue }
    if (id === 'listings') { await press(/Leave it off/, 'main'); await wait(500); continue }
    await onward()
  }
  await until(`location.pathname === '/setup/done'`, 20000, 'the last page')
  await wait(1500)
  await maskAddresses()
  await card(`setup-${n++}-done`, CARD)
  console.log(await page.eval(`document.querySelector('main').innerText`))
  await press(/Start watching/)
  await until(`!location.pathname.startsWith('/setup')`, 15000, 'leaving setup')
  await wait(2500)
  await shot('setup-99-home')
  console.log('saveMetadata', (await s.call('GET', '/settings'))?.saveMetadata)
} catch (e) {
  console.log('STOPPED', e.message, await page.eval('location.pathname'))
  await shot('setup-stopped').catch(() => {})
  process.exitCode = 1
} finally {
  page.close()
  await s.browser.close()
}
