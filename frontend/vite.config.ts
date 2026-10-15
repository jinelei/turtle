import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Wails 在 wails dev 时会注入相关环境变量。
export default defineConfig({
  plugins: [vue()],
  clearScreen: false,
  server: {
    strictPort: true,
    port: 5173,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
