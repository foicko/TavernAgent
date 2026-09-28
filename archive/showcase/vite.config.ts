import { resolve } from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// 展厅直接复用 Web 端的设计令牌（web/src/styles/tokens.css），保证展示的就是真实系统。
const webTokens = resolve(__dirname, "../web/src/styles/tokens.css");

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@tokens": webTokens,
    },
  },
  server: {
    port: 5174,
    host: true,
    fs: {
      // 允许开发服务器读取仓库根下的 web/ 令牌文件
      allow: [resolve(__dirname, "..")],
    },
  },
});
