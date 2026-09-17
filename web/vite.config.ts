import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    host: "0.0.0.0",
    port: 5173,
    proxy: {
      // 允许用 VITE_API_TARGET 指向另一个后端实例（性能基线要在数据集上测，
      // 不能拿正在使用的数据目录当被测对象）。
      "/api": {
        target: process.env.VITE_API_TARGET ?? "http://127.0.0.1:8890",
        changeOrigin: true,
      },
    },
  },
  build: {
    target: "es2022",
  },
});