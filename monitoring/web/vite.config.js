/** @type {import('vite').UserConfig} */
export default {
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:42217",
        changeOrigin: true,
      },
    },
  },
};
