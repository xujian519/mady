/**
 * 组件测试全局 setup（vitest.component.config.ts 引用）。
 *
 * 引入 @testing-library/jest-dom 的自定义 matcher
 * （toBeInTheDocument / toHaveClass / toBeDisabled 等），
 * 使组件测试断言更贴近用户可见行为。
 *
 * 注意：必须使用 `/vitest` 专用入口——jest-dom v7 默认入口按 jest 的
 * 全局 expect 设计（expect.extend 在模块顶层执行），在 vitest 下会
 * 报 `expect is not defined`；`/vitest` 入口与 vitest 的 expect 集成。
 */
import '@testing-library/jest-dom/vitest'
