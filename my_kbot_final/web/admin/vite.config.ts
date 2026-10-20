import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  base: '/',
  plugins: [react()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: {
    host: '127.0.0.1',
    port: 15173,
    strictPort: true,
    proxy: {
      '/healthz': { target: 'http://127.0.0.1:18090', changeOrigin: true },
      '/readyz': { target: 'http://127.0.0.1:18090', changeOrigin: true },
      '/api': { target: 'http://127.0.0.1:18090', changeOrigin: true },
      '/stream': { target: 'http://127.0.0.1:18090', changeOrigin: true },
    },
  },
  build: { outDir: 'dist', sourcemap: false },
})
