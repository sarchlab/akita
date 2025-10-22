/** @type {import('vite').UserConfig} */
export default {
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:38409",
        changeOrigin: true,
      },
    },
  },
};
