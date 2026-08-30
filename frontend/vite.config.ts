import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { biotronFavicon } from '@biotron/style/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss(), biotronFavicon()],
  envDir: '..',
  server: {
    // bind 0.0.0.0 so the port is reachable from the host (dev container / docker)
    host: true,
    port: 5173,
  },
})
