import { fileURLToPath, URL } from "node:url";

import { defineConfig } from "vite";

const htmlInput = (fileName) => fileURLToPath(new URL(fileName, import.meta.url));

export default defineConfig({
  build: {
    rollupOptions: {
      input: {
        index: htmlInput("index.html"),
        datavisualization: htmlInput("datavisualization.html"),
      },
    },
    sourcemap: true,
  },
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:3001",
        changeOrigin: true,
      },
    },
  },
});
