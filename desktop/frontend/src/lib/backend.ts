/**
 * 后端服务层（聚合入口）。
 *
 * 封装对 Wails Go Binding 的调用，提供类型安全的 API。
 * 生产环境优先使用 Wails 注入的 `window.go` 全局对象；
 * 开发期/浏览器回退到动态导入生成的 `wailsjs` 模块。
 *
 * 实现按领域拆在 backend/ 下（M60）：runtime 负责绑定解析与运行态信息，
 * conversation 负责会话/标签/记忆，workspace 负责文件/技能/MCP/知识库/项目。
 * 本文件只做 re-export，所有既有 `@/lib/backend` 导入保持不变。
 */

export * from './backend/runtime'
export * from './backend/conversation'
export * from './backend/workspace'
