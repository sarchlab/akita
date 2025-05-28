/** @type {import('vite').UserConfig} */
export default {
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:36219",
        changeOrigin: true,
      },
    },
  },
};
