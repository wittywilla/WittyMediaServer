// Keeps web/module-manifests in step with the module repositories: the server embeds these
// (web/embed.go), and shouldn't need Node to build. Go modules have their manifest in Go.
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'

const web = resolve(import.meta.dirname, '..')
const installed = join(web, 'node_modules/@wittywilla')
const out = join(web, 'module-manifests')
rmSync(out, { recursive: true, force: true })
mkdirSync(out)
for (const id of readdirSync(installed)) {
  const manifest = join(installed, id, 'module.json')
  if (existsSync(manifest) && !existsSync(join(installed, id, 'go.mod'))) {
    writeFileSync(join(out, id + '.json'), readFileSync(manifest, 'utf8').replace(/\r\n/g, '\n'))
  }
}
