/** @type {import('vite').UserConfig} */
export default {
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:36987",
        changeOrigin: true,
      },
    },
  },
};
