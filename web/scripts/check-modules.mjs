// Fails the build if a module reaches into app code. A module under
// src/modules/<name> may import only its own files, the libraries listed
// below and the plugins it builds on, by package name (@wittywilla/<name>);
// everything else comes through its host interface. See docs/MODULES.md.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, resolve, sep } from 'node:path'

const LIBS = { wittypdf: ['react', 'pdfjs-dist'] }
const USES = { wittyebook: ['wittyink'], wittypdf: ['wittyink'], wittycomics: ['wittyink'] }
const root = resolve(import.meta.dirname, '../src/modules')
const files = (dir) => readdirSync(dir).flatMap((f) => {
  const p = join(dir, f)
  return statSync(p).isDirectory() ? files(p) : /\.(tsx?|css)$/.test(f) ? [p] : []
})

let bad = 0
for (const mod of readdirSync(root)) {
  const dir = join(root, mod)
  if (!statSync(dir).isDirectory()) continue
  const libs = LIBS[mod] || ['react']
  const uses = (USES[mod] || []).map((m) => '@wittywilla/' + m)
  for (const f of files(dir)) {
    const src = readFileSync(f, 'utf8')
    for (const m of src.matchAll(/(?:import|export)[^'"]*?from\s*['"]([^'"]+)['"]|import\s*\(\s*['"]([^'"]+)['"]\s*\)|@import\s+['"]([^'"]+)['"]|^\s*import\s+['"]([^'"]+)['"]/gm)) {
      const spec = m[1] || m[2] || m[3] || m[4]
      const ok = spec.startsWith('.')
        ? (resolve(f, '..', spec) + sep).startsWith(dir + sep)
        : uses.includes(spec) || libs.some((l) => spec === l || spec.startsWith(l + '/'))
      if (!ok) { bad++; console.error(`${relative(process.cwd(), f)}: module "${mod}" imports "${spec}" (allowed: its own files, ${[...libs, ...(USES[mod] || [])].join(', ')})`) }
    }
  }
}
if (bad) process.exit(1)
console.log('modules: ok')
