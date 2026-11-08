import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';
import { biotronFavicon } from '@biotron/style/vite';

export default defineConfig({
  plugins: [react(), tailwindcss(), biotronFavicon()],
  envDir: '..',
  server: { host: true, port: 5176 },
  test: {
    // Every date helper names its time zone, but run in another one to prove it.
    env: { TZ: 'Asia/Tokyo' },
  },
});
