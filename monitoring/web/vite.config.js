/** @type {import('vite').UserConfig} */
export default {
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:33767",
        changeOrigin: true,
      },
    },
  },
};
