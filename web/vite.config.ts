import { readFileSync, readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
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

export default defineConfig({
  plugins: [react()],
  define: { __WEB_MODULES__: JSON.stringify(webModules) },
  // Plugins (src/modules/*) are imported by their package names, as in their
  // own repositories (docs/MODULES.md).
  resolve: { alias: [{ find: /^@wittywilla\/(witty[a-z]+)(.*)$/, replacement: fileURLToPath(new URL('./src/modules/', import.meta.url)) + '$1$2' }] },
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false },
  server: {
    port: 5173,
    proxy: { '/api': { target: 'http://127.0.0.1:9797', changeOrigin: false } },
  },
})
