import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [react()],
  envDir: '..',
  server: {
    host: true,
    port: 5178,
  },
});
