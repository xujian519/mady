# Phase 5: P3 领域扩展与基础设施审阅报告

> 日期：2026-07-14
> 审阅范围：guardrails/guardian/ + knowledge/ + disclosure/ + graph/ + workflow/ + retrieval/ + psychological/ + memory/compiler/

## 审计总结

| 模块 | Critical | High | Medium | Low | 状态 |
|------|----------|------|--------|-----|------|
| guardrails/guardian/ | 0 | 1 | 3 | 0 | 🟡 |
| knowledge/ | 0 | 2 | 3 | 1 | 🟡 |
| disclosure/ | 0 | 0 | 1 | 1 | 🟢 |
| graph/ | 0 | 0 | 1 | 1 | 🟢 |
| workflow/ + workflows/ | 0 | 1 | 3 | 0 | 🟡 |
| retrieval/ | 0 | 0 | 1 | 1 | 🟢 |
| psychological/ | 0 | 0 | 1 | 2 | 🟢 |
| memory/compiler/ | 0 | 0 | 1 | 1 | 🟢 |
| **合计** | **0** | **4** | **14** | **7** | **31** |

---

## 🔴 Critical 发现

**无。** 领域扩展和基础设施层没有 Critical 级别问题。

## 🟠 High 发现（4个）

| # | 文件 | 问题 | 建议 |
|---|------|------|------|
| H1 | `guardrails/guardian/circuitbreaker.go:16-20` | **CircuitBreaker 无并发保护** — RecordDenial/RecordAllow/IsTripped 无锁 | 加 sync.Mutex |
| H2 | `knowledge/fileindex/store.go:631,653` | **全量文件读取** — extractPreview/quickChecksum 使用 os.ReadFile 读整个文件 | io.ReadFull(r, buf[:512]) 只读所需 |
| H3 | `knowledge/fileindex/store.go:168-169` | **rows.Err() 被忽略** — Refresh 中 rows.Close() 后未检查错误 | 添加 rows.Err() 检查 |
| H4 | `workflows/` 顶层包 | **零测试** — extractFeatures/buildSearchQuery 等核心函数无测试 | 补充测试 |

## 🟡 Medium 关键发现

- **CircuitBreaker 熔断后永不恢复**（circuitbreaker.go:33） — tripped=true 后无自动 untrip
- **parseAssessment fallback 关键词过宽**（assessment.go:60） — "deny" 子串匹配
- **vecIndex 未同步访问**（store.go:76,191） — 需 atomic.Pointer
- **Refresh 持有锁时执行 I/O**（fileindex/store.go:148） — 可能阻塞所有搜索
- **RemoveEdges 全图扫描**（graph/store.go:124-129） — sourceID/targetID 已指定时多余
- **EmoRelief 与 EmoJoy 公式完全相同**（occ.go:9,23） — 应为不同权重
- **Parallel.Run 缺乏 panic 保护**（workflow.go:69-78）
- **Pregel 每次超步完整序列化**（pregel.go:286-357） — 大状态时性能问题

## 已知问题跟踪

| 编号 | 问题 | 评估结果 |
|------|------|---------|
| #7 | unsafe.Slice 向量解析 | ✅ **安全** — append 在下次循环前已复制数据 |
| #8 | sync.Pool 缺失 | **确认** — vectorIndex.Search 每调用分配 worker 切片 |

---

## 整体评估

P3 领域扩展层状态：**良好**。无 Critical 安全问题。主要改进方向：CircuitBreaker 并发安全、文件索引性能优化、workflows 测试补充。心理引擎 OCC 公式权重需对齐理论。
