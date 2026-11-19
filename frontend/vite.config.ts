import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // Same origin in dev, so the session cookie and CSRF work without CORS.
    proxy: {
      '/api': process.env.VITE_API_TARGET ?? 'http://localhost:8080',
    },
  },
})
