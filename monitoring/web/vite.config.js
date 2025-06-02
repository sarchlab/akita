/** @type {import('vite').UserConfig} */
export default {
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:42247",
        changeOrigin: true,
      },
    },
  },
};
