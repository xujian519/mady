import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'
import fs from 'fs'

/**
 * build 时将 pdfjs-dist 的 cmaps/ 复制到 dist/cmaps/，
 * 供 PdfViewer (pdf.js) 的 cMapUrl 在运行时加载 CJK CMap。
 */
function copyCmapsPlugin() {
  return {
    name: 'copy-cmaps',
    closeBundle() {
      const src = path.resolve(__dirname, 'node_modules/pdfjs-dist/cmaps')
      const dest = path.resolve(__dirname, 'dist/cmaps')
      fs.cpSync(src, dest, { recursive: true })
    },
  }
}

export default defineConfig({
  plugins: [react(), tailwindcss(), copyCmapsPlugin()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  // Wails v2 dev server 代理：开发模式由 Wails 管理，此处仅为类型检查
  server: {
    port: 5173,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // 禁用 sourcemap 缩小二进制体积
    sourcemap: false,
  },
})
