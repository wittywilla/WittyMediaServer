import { copyFileSync, existsSync, mkdirSync, readFileSync, readdirSync, realpathSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { gzipSync } from 'node:zlib'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'

// The reader modules' versions, from their package.json files, for
// Settings, About.
const modulesDir = fileURLToPath(new URL('./node_modules/@wittywilla/', import.meta.url))
const webModules = readdirSync(modulesDir).flatMap((d) => {
  // The Go modules' web halves aren't modules of the web app.
  if (!existsSync(modulesDir + d + '/module.json') || existsSync(modulesDir + d + '/go.mod')) return []
  try {
    const p = JSON.parse(readFileSync(modulesDir + d + '/package.json', 'utf8'))
    return [{ id: d, version: String(p.version || 'dev'), description: String(p.description || '') }]
  } catch { return [] }
})

// Writes a .gz beside each script and stylesheet: the server sends these to
// browsers that take them, with no compressing work of its own (see spa() in
// internal/server).
function precompress(): Plugin {
  return {
    name: 'precompress',
    apply: 'build',
    closeBundle() {
      const dir = fileURLToPath(new URL('./dist/assets/', import.meta.url))
      for (const f of readdirSync(dir)) {
        if (!/\.(js|mjs|css)$/.test(f)) continue
        const raw = readFileSync(dir + f)
        if (raw.length > 1024) writeFileSync(dir + f + '.gz', gzipSync(raw, { level: 9 }))
      }
    },
  }
}

// pdf.js fetches its image decoders and colour profile by name, from a folder it
// is told about, so they can't go through the bundler. The folder carries pdf.js's
// version because everything under assets/ is cached for good. The script engine
// for PDF scripts stays behind: PDF scripts never run here.
const pdfVersion = String(JSON.parse(readFileSync(new URL('./node_modules/pdfjs-dist/package.json', import.meta.url), 'utf8')).version)
function pdfAssets(): Plugin {
  return {
    name: 'pdf-assets',
    apply: 'build',
    closeBundle() {
      for (const [from, match] of [['wasm', /^(openjpeg|jbig2|qcms).*\.(wasm|js)$/], ['iccs', /\.icc$/]] as const) {
        const src = fileURLToPath(new URL(`./node_modules/pdfjs-dist/${from}/`, import.meta.url))
        const dst = fileURLToPath(new URL(`./dist/assets/pdfjs-${pdfVersion}/${from}/`, import.meta.url))
        mkdirSync(dst, { recursive: true })
        for (const f of readdirSync(src)) if (match.test(f)) copyFileSync(src + f, dst + f)
      }
    },
  }
}

export default defineConfig({
  plugins: [react(), precompress(), pdfAssets()],
  define: { __WEB_MODULES__: JSON.stringify(webModules), __PDF_ASSETS__: JSON.stringify(`/assets/pdfjs-${pdfVersion}/`) },
  // The modules are packages linked into node_modules, as TypeScript source
  // (scripts/dev-modules.mjs). dedupe resolves what they import from here.
  resolve: {
    dedupe: ['react', 'react-dom', 'pdfjs-dist', '@wittywilla/wittyink', '@wittywilla/wittyebook', '@wittywilla/wittypdf', '@wittywilla/wittycomics', '@wittywilla/wittyvisualizer', '@wittywilla/wittyplay', '@wittywilla/wittyintegratedfilebrowser', '@wittywilla/wittycouch'],
  },
  // The browsers this app has always reached, TVs included, whatever the bundler's default is now.
  optimizeDeps: { exclude: ['@wittywilla/wittyink', '@wittywilla/wittyebook', '@wittywilla/wittypdf', '@wittywilla/wittycomics', '@wittywilla/wittyvisualizer', '@wittywilla/wittyplay', '@wittywilla/wittyintegratedfilebrowser', '@wittywilla/wittycouch'] },
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false, target: ['es2020', 'edge88', 'firefox78', 'chrome87', 'safari14'] },
  server: {
    port: 5173,
    proxy: { '/api': { target: process.env.WMS_API || 'http://127.0.0.1:9797', changeOrigin: false } },
    fs: { allow: ['..', ...readdirSync(modulesDir).map((d) => realpathSync(modulesDir + d))] }, // the modules' repositories
  },
})
