import { defineConfig } from 'vitest/config'
import path from 'path'

/**
 * Vitest 组件测试专用配置（jsdom 环境 + jest-dom matchers）。
 *
 * 独立于 vitest.config.ts：纯逻辑与 store 测试跑 node 环境（.test.ts），
 * 组件测试（.test.tsx）跑 jsdom 环境。两者配置分离，避免：
 * - node 环境下 .tsx 组件测试依赖 DOM API 失败
 * - 每个组件测试文件手写 `@vitest-environment jsdom` 注释
 *
 * jest-dom 的扩展 matcher（toBeInTheDocument / toHaveClass 等）由
 * src/test/setup.ts 统一引入，组件测试可直接使用。
 */
export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.tsx'],
    setupFiles: ['./src/test/setup.ts'],
  },
})
