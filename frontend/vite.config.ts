import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const NGROK_METRICS =
  process.env.VITE_API_URL?.replace(/\/$/, '') ||
  'https://uncomfortably-unshut-jaclyn.ngrok-free.dev'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: NGROK_METRICS,
        changeOrigin: true,
        secure: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
        configure: (proxy) => {
          proxy.on('proxyReq', (proxyReq) => {
            proxyReq.setHeader('ngrok-skip-browser-warning', 'true')
          })
        },
      },
    },
  },
})
