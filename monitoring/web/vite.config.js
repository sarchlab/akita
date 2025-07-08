/** @type {import('vite').UserConfig} */
export default {
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:36467",
        changeOrigin: true,
      },
    },
  },
};
