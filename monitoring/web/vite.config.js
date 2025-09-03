/** @type {import('vite').UserConfig} */
export default {
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:37857",
        changeOrigin: true,
      },
    },
  },
};
