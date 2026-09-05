import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';
import { biotronFavicon } from '@biotron/style/vite';

export default defineConfig({
  plugins: [react(), tailwindcss(), biotronFavicon()],
  envDir: '..',
  server: {
    host: true,
    port: 5178,
  },
});
