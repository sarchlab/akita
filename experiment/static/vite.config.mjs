import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const monitorTarget = process.env.EXP_MONITOR_URL ?? "http://localhost:8765";

export default defineConfig({
  plugins: [react()],
  build: {
    sourcemap: false,
  },
  server: {
    host: "localhost",
    proxy: {
      "/api": {
        target: monitorTarget,
        changeOrigin: true,
      },
    },
  },
});
