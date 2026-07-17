import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const wsPort = process.env.VITE_WS_PORT || '3972'

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
        target: `http://localhost:${wsPort}`,
        changeOrigin: true,
        ws: true,
      },
      '/login': {
        target: `http://localhost:${wsPort}`,
        changeOrigin: true,
      },
      '/common': {
        target: `http://localhost:${wsPort}`,
        changeOrigin: true,
      },
      '/ws': {
        target: `ws://localhost:${wsPort}`,
        ws: true,
      },
    },
  },
})
