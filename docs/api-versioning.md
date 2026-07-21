# API 版本化策略

> 本文档定义 Mady Server 的 API 版本化方案与兼容性承诺。
> 首次建立于 2026-07-21，随 API 变更同步更新。

## 版本方案

- **当前版本**: v1（2026-07-21 稳定）
- 所有端点通过 URL 前缀区分版本：`/api/v1/*`
- 旧版无版本前缀路径（`/api/*`）保持可用，但标记为已废弃
- Disclosure API 从设计之初即为 `/v1/disclosure/*`，视为 v1 版本

## 端点清单

### v1（稳定版，GA）

| 方法 | 路径 | 状态 | 说明 |
|------|------|------|------|
| POST | /api/v1/chat | GA | 发送消息并获取 Agent 响应（同步/SSE） |
| GET  | /api/v1/skills | GA | 列出已注册技能 |
| GET  | /api/v1/skills/diagnostics | GA | 获取技能加载诊断信息 |
| GET  | /api/v1/skills/events | GA | SSE 流式获取技能事件 |
| GET  | /api/v1/skills/status | GA | 获取技能注册表状态 |
| POST | /api/v1/skills/reload | GA | 热重载技能 |
| POST | /v1/disclosure/analyze | GA | 提交技术交底书分析 |
| GET  | /v1/disclosure/analyze/{task_id} | GA | 轮询分析任务状态 |
| GET  | /v1/disclosure/analyze/{task_id}/stream | GA | SSE 流式获取分析进度 |
| POST | /v1/disclosure/analyze/{task_id}/review | GA | 提交人工复核结论 |
| POST | /api/v1/threads | GA | 创建新线程 |
| GET  | /api/v1/threads | GA | 列出所有线程 |
| GET  | /api/v1/threads/{key} | GA | 获取线程详情 |
| GET  | /api/v1/threads/{key}/config | GA | 获取线程级调用配置 |
| PUT  | /api/v1/threads/{key}/config | GA | 设置线程级调用配置 |
| GET  | /api/v1/threads/{key}/thinking | GA | 获取线程级推理配置 |
| PUT  | /api/v1/threads/{key}/thinking | GA | 设置线程级推理配置 |
| POST | /api/v1/threads/{key}/branch | GA | 从指定节点创建分支 |
| DELETE | /api/v1/threads/{key} | GA | 删除线程 |
| GET  | /api/v1/states | GA | 列出持久化状态键 |
| GET  | /api/v1/states/{key} | GA | 获取状态快照 |
| DELETE | /api/v1/states/{key} | GA | 删除状态快照 |

### 运维端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /health | 存活检查（轻量，用于负载均衡） |
| GET | /ready | 就绪检查（Agent 池 + Disclosure 健康度） |
| GET | /metrics | Prometheus 格式指标（基于 expvar） |
| GET | /debug/vars | expvar 调试指标 |

### 非版本化端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /agui/{path} | Agent GUI 事件流（非 REST，保持独立） |

## 响应头

| 响应头 | 说明 |
|--------|------|
| X-API-Version | 端点版本号，如 `v1` |
| X-API-Deprecated | 旧版路径标记为 `true` |
| X-API-Deprecated-Date | 废弃标注日期 |
| X-Request-ID | 请求追踪 ID（loggingMiddleware 注入） |

## 兼容性承诺

| 状态 | 含义 | 向后兼容保证 |
|------|------|-------------|
| GA | General Availability，生产就绪 | 保证 **6 个月** 向后兼容 |
| Beta | 公测中，可能变更 | 标注 `X-API-Status: beta`，不保证兼容 |
| Deprecated | 已废弃 | 保留 **3 个月** 后移除 |

### 变更管理

1. 新增端点一律使用 `/api/v{N}/` 前缀
2. 不兼容变更必须增加版本号（v1 → v2）
3. 废弃端点必须：
   - 在响应头添加 `X-API-Deprecated: true`
   - 在文档标记 `deprecated: true`
   - 保留至少 3 个月后删除
4. 向后兼容的变更（新增字段、新增端点）可在当前版本内进行

## 迁移指南（v0 → v1）

### 旧的 v0 消费者

所有 `/api/*` 路径在 v1 发布后继续可用，但响应头会增加 `X-API-Deprecated: true`。

推荐迁移步骤：
1. 将请求路径从 `/api/chat` 改为 `/api/v1/chat`
2. 验证响应头包含 `X-API-Version: v1`
3. 检查响应体结构无变化（当前版本为纯加法变更）
4. 通知 API 消费者在 3 个月内完成迁移

## 设计决策

### 为什么选择 URL 前缀而非 Accept header？

- URL 前缀更直观，便于调试和文档查阅
- Go 1.26 ServeMux 原生支持 `METHOD /path` 模式匹配，前缀替换实现简单
- 浏览器/curl 等工具无需设置 Accept header
- 与现有 `/v1/disclosure/*` 设计一致

### 旧版路径保留策略

旧版 `/api/*` 路径保留为路由别名，而非内部重定向（302），原因：
- 避免增加一次 HTTP 往返
- 避免改变 Go ServeMux 的 handler 调用链
- 旧版 handler 逻辑完全不变，只增加响应头标注
