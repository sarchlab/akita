/** @type {import('vite').UserConfig} */
export default {
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:37521",
        changeOrigin: true,
      },
    },
  },
};
