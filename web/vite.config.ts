import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Build straight into the Go embed directory so `make web && make mathiz`
// produces a single self-contained binary.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../internal/saas/webui/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        ws: true,
      },
    },
  },
})
