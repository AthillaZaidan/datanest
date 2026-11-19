import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

const target = process.env.VITE_API_TARGET ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // The browser talks to this dev server only, so the session cookie (SameSite=Strict) and
    // CSRF work without CORS. The backend accepts a single Origin, so present it as its own.
    proxy: {
      '/api': {
        target,
        configure: (proxy) => proxy.on('proxyReq', (req) => req.setHeader('Origin', target)),
      },
    },
  },
})
