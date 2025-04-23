/** @type {import('vite').UserConfig} */
export default {
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:52978",
        changeOrigin: true,
      },
    },
  },
};
