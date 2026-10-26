import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';
import { biotronFavicon } from '@biotron/style/vite';

export default defineConfig({
  plugins: [react(), biotronFavicon()],
  envDir: '..',
  server: {
    host: true,
    port: 5175,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8082',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
});
