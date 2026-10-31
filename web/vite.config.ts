import { readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { gzipSync } from 'node:zlib'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'

// The reader modules' versions, from their package.json files, for
// Settings, About.
const modulesDir = fileURLToPath(new URL('./src/modules/', import.meta.url))
const webModules = readdirSync(modulesDir).flatMap((d) => {
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

export default defineConfig({
  plugins: [react(), precompress()],
  define: { __WEB_MODULES__: JSON.stringify(webModules) },
  // Plugins (src/modules/*) are imported by their package names, as in their
  // own repositories (docs/MODULES.md).
  // WittyIntegratedFileBrowser keeps its web half beside its Go half
  // (modules/wittyintegratedfilebrowser/web); dedupe resolves React for it
  // from this folder.
  resolve: {
    alias: [
      { find: /^@wittywilla\/wittyintegratedfilebrowser$/, replacement: fileURLToPath(new URL('../modules/wittyintegratedfilebrowser/web/index.ts', import.meta.url)) },
      { find: /^@wittywilla\/wittyplay$/, replacement: fileURLToPath(new URL('../modules/wittyplay/web/index.ts', import.meta.url)) },
      { find: /^@wittywilla\/wittycouch$/, replacement: fileURLToPath(new URL('../modules/wittycouch/web/index.ts', import.meta.url)) },
      { find: /^@wittywilla\/(witty[a-z]+)(.*)$/, replacement: fileURLToPath(new URL('./src/modules/', import.meta.url)) + '$1$2' },
    ],
    dedupe: ['react', 'react-dom'],
  },
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false },
  server: {
    port: 5173,
    proxy: { '/api': { target: 'http://127.0.0.1:9797', changeOrigin: false } },
    fs: { allow: ['..'] }, // the module's web half lives outside web/
  },
})
