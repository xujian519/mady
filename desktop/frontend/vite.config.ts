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

/**
 * dev server 专用：放宽 CSP script-src 允许内联脚本。
 * Vite dev 模式会注入内联 react-refresh preamble，
 * 生产构建的 CSP（script-src 'self'）保持不变。
 */
function devCspPlugin() {
  return {
    name: 'dev-csp',
    apply: 'serve' as const,
    transformIndexHtml(html: string) {
      return html.replace(
        "script-src 'self'",
        "script-src 'self' 'unsafe-inline'",
      )
    },
  }
}

export default defineConfig({
  plugins: [react(), tailwindcss(), copyCmapsPlugin(), devCspPlugin()],
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
    // 拆分大块：pdf.js (~3MB)、CodeMirror (~500KB)、React 框架与 UI 工具
    // 各自独立 chunk，避免单 entry 超过 500KB 拖慢首屏加载；
    // vendor 块独立缓存，业务代码更新时框架层不必重新下载。
    rollupOptions: {
      output: {
        manualChunks: {
          pdfjs: ['pdfjs-dist'],
          codemirror: [
            '@codemirror/view',
            '@codemirror/state',
            '@codemirror/language',
            '@codemirror/commands',
            '@codemirror/theme-one-dark',
            '@codemirror/lang-markdown',
          ],
          react: ['react', 'react-dom', 'zustand', 'framer-motion', '@tanstack/react-query', '@tanstack/react-virtual'],
          // lucide-react 图标库独立缓存：minified ~700KB / gzip ~135KB，
          // 超过 500KB 的 Vite 警告可接受（图标更新频率远低于业务代码）。
          ui: ['lucide-react'],
        },
      },
    },
  },
})
