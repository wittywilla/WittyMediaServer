import { readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { gzipSync } from 'node:zlib'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'

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
  define: { __WEB_MODULES__: JSON.stringify([]) },
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false },
  server: {
    port: 5173,
    proxy: { '/api': { target: process.env.WMS_API || 'http://127.0.0.1:9797', changeOrigin: false } },
  },
})
