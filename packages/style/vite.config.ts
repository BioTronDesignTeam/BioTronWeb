import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  server: { host: true, port: 5179 },
  test: {
    // The theme cookie and the session provider need a DOM.
    environment: "jsdom",
    restoreMocks: true,
    unstubGlobals: true,
  },
  build: {
    lib: {
      entry: new URL("./src/index.ts", import.meta.url).pathname,
      name: "BioTronStyle",
      fileName: "biotron-style",
      cssFileName: "biotron-style",
    },
    rollupOptions: {
      external: ["react", "react-dom", "react/jsx-runtime"],
      output: {
        globals: { react: "React", "react-dom": "ReactDOM", "react/jsx-runtime": "jsxRuntime" },
      },
    },
  },
});
