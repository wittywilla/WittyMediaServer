// Drives tests/gallery.html in a headless browser. Start a Vite dev server first:
//   (cd web && npx vite --port 5199)
//   node tests/gallery-run.mjs sheet out.png            a contact sheet of every effect
//   node tests/gallery-run.mjs sheet out.png 480 270 8 skyline,hearth
//   node tests/gallery-run.mjs speed [--software]       milliseconds per frame, per effect
//   node tests/gallery-run.mjs flash [--software]       strobe through the real guard
import { writeFileSync } from 'node:fs'
import { launch } from './cdp.mjs'
import { worstFlashesPerSecond } from '../safety.ts'

const args = process.argv.slice(2)
const software = args.includes('--software')
const [cmd, ...rest] = args.filter((a) => a !== '--software')
const URL_ = process.env.WV_GALLERY || 'http://localhost:5199/src/modules/wittyvisualizer/tests/gallery.html'

const b = await launch({ software, tmp: process.env.WV_TMP, width: 2000, height: 1200 })
try {
  const p = await b.open(URL_)
  await p.until('document.title === "ready"', 30000, 'the gallery').catch((e) => { console.log(p.logs.join('\n')); throw e })
  console.log('graphics:', await p.eval('gallery.gpu'))
  const compiled = await p.eval('gallery.compileAll()')
  const bad = Object.entries(compiled).filter(([, v]) => !v.startsWith('ok'))
  if (bad.length) { console.log('would not compile:'); for (const [k, v] of bad) console.log(k, v); process.exitCode = 1 }
  else console.log('all compile:', Object.entries(compiled).map(([k, v]) => `${k} ${v.slice(3)}`).join(', '))
  if (cmd === 'sheet') {
    const [out, w = 400, h = 225, secs = 6, ids] = rest
    const only = ids ? JSON.stringify(ids.split(',')) : 'undefined'
    const url = await p.eval(`gallery.sheet(${w}, ${h}, ${secs}, ${only})`)
    writeFileSync(out, Buffer.from(url.split(',')[1], 'base64'))
    console.log('wrote', out)
  } else if (cmd === 'speed') {
    const sizes = software ? [[640, 360]] : [[1920, 1080], [1280, 720]]
    for (const [w, h] of sizes) {
      const ids = await p.eval('gallery.ids')
      const row = []
      for (const id of ids) row.push(`${id} ${await p.eval(`gallery.speed(${JSON.stringify(id)}, ${w}, ${h}, ${software ? 20 : 60})`)}`)
      console.log(`${w}x${h} ms per frame:`, row.join(', '))
    }
  } else if (cmd === 'flash') {
    for (const calm of [false, true]) {
      for (const hz of [2, 3, 4, 6, 8, 10, 15, 25]) {
        const { values, times } = await p.eval(`gallery.strobe(${hz}, ${calm})`)
        const n = worstFlashesPerSecond(values, times)
        console.log(`${calm ? 'calm  ' : 'normal'} strobe at ${String(hz).padStart(2)} Hz: worst second has ${n} flash${n === 1 ? '' : 'es'}, brightness ${Math.min(...values.slice(60)).toFixed(3)} to ${Math.max(...values.slice(60)).toFixed(3)}`)
        if (n > 3) process.exitCode = 1
      }
    }
  }
  const errs = p.logs.filter((l) => /^(error|exception)/.test(l))
  if (errs.length) console.log(errs.join('\n'))
  p.close()
} finally {
  await b.close()
}
