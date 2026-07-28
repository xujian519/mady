import { defineConfig } from 'vitest/config'
import path from 'path'

/**
 * Vitest 单元测试配置。
 *
 * 独立于 vite.config.ts：单测只覆盖纯函数与 store 逻辑，
 * 不需要 React/Tailwind 插件与 pdf.js cmaps 拷贝，
 * 避免 vitest 自带 vite 版本与构建插件的兼容性问题。
 */
export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
  },
})
