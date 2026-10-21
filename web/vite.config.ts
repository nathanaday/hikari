import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// In development, `npm run dev` serves the UI and sends API requests to a
// running `hikari up`.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': path.resolve(import.meta.dirname, 'src') },
  },
  build: {
    // The hikari binary serves the bundle from localhost, so its size matters little.
    chunkSizeWarningLimit: 1024,
  },
  server: {
    proxy: { '/api': 'http://127.0.0.1:7420' },
  },
})
