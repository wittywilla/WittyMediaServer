import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  // Plugins (src/modules/*) are imported by their package names, as in their
  // own repositories (docs/MODULES.md).
  resolve: { alias: [{ find: /^@wittywilla\/(witty[a-z]+)(.*)$/, replacement: fileURLToPath(new URL('./src/modules/', import.meta.url)) + '$1$2' }] },
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false },
  server: {
    port: 5173,
    proxy: { '/api': { target: 'http://127.0.0.1:9797', changeOrigin: false } },
  },
})
