/** @type {import('vite').UserConfig} */
export default {
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:39325",
        changeOrigin: true,
      },
    },
  },
};
