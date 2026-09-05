import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { biotronFavicon } from '@biotron/style/vite';
import { fileURLToPath, URL } from 'node:url';

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), biotronFavicon()],
  envDir: '..',
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  // 5177 belongs to the always-on production-shaped preview container, so the
  // dev server takes 5180 instead. With strictPort both would otherwise fight
  // over the same bind and `npm run dev` would simply fail.
  server: {
    host: true,
    port: 5180,
    strictPort: true,
  },
  preview: {
    host: true,
    port: 4173,
    strictPort: true,
  },
  build: {
    target: 'es2020',
    rolldownOptions: {
      output: {
        // Vite 8 bundles with Rolldown, which dropped the object form of
        // `manualChunks`. The same three chunks come back as ordered groups.
        // `three` is declared first so the renderer keeps its own chunk instead
        // of being pulled into the wrappers that import it.
        codeSplitting: {
          groups: [
            { name: 'three', test: /node_modules[\\/]three[\\/]/ },
            { name: 'r3f', test: /node_modules[\\/](@react-three|three-stdlib|postprocessing)[\\/]/ },
            { name: 'gsap', test: /node_modules[\\/](gsap|@gsap)[\\/]/ },
          ],
        },
      },
    },
  },
});
