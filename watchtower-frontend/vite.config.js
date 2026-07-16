import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  base: '/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    // 开发时将所有 /api/、/common/、/login、/ws 请求代理到 Go 后端
    proxy: {
      '/api': {
        target: 'http://localhost:3972',
        changeOrigin: true,
      },
      '/login': {
        target: 'http://localhost:3972',
        changeOrigin: true,
      },
      '/common': {
        target: 'http://localhost:3972',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://localhost:3972',
        ws: true,
      },
    },
  },
})
