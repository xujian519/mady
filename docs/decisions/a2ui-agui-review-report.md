# A2UI / AGUI 模块全量质量审阅报告

**日期**: 2026-07-27
**审阅范围**: `a2ui/`（14 .go + 2 _test + README，~3,900 行） + `agui/`（4 .go + 3 _test + TS SDK + 协议文档，~3,700 行）
**审阅方法**: 5 阶段（静态分析 → 竞态/覆盖率 → 8 维度人工审阅 → 集成验证 → 报告生成）

---

## 评分汇总

| 维度 | A2UI | AGUI | 综合 | 评估 |
|------|------|------|------|------|
| 1. 正确性 | 8 | 7 | **7.5** | 2 个 P0 bug 需立即修复 |
| 2. 安全性 | 7 | 6 | **6.5** | SystemPrompt 泄露，callConfig stub |
| 3. 性能 | 8 | 7 | **7.5** | 无明显瓶颈，少量 P3 观察点 |
| 4. 可维护性 | 7 | 6 | **6.5** | Convert 162 行重复 switch，TS 无同步 |
| 5. 测试质量 | 10 | 6 | **8.0** | A2UI 100% ✅，AGUI 77.7% ❌ |
| 6. 集成正确性 | 7 | 7 | **7.0** | binding 错误处理有缺陷 |
| 7. 错误处理 | 7 | 6 | **6.5** | 同 P0-1/P0-2，静默吞没错误 |
| 8. 并发安全 | 7 | 8 | **7.5** | 整体良好，Converter 原子操作冗余 |

**综合评分: 7.1/10** — 代码质量总体良好，但存在 2 个 P0 阻塞性问题需优先修复。

---

## 问题清单（按优先级排序）

### P0（2 项）— 必须修复

| # | 文件:行 | 问题 | 维度 |
|---|---------|------|------|
| P0-1 | `agui/handler.go:67,103-105` | **LoadAgent 失败时 Content-Type 冲突** — 67 行设 `text/event-stream`，但 104 行 `writeJSON` 改为 `application/json` | 正确性, 错误处理 |
| P0-2 | `agui/handler.go:183-189` | **agent.Run 错误时不发 RUN_ERROR** — 仅发 closeAll + RUN_FINISHED，前端无法感知失败 | 正确性, 错误处理 |

### P1（7 项）— 强烈建议修复

| # | 文件:行 | 问题 | 维度 |
|---|---------|------|------|
| P1-1 | `agui/handler.go:192-197` | **callConfigFromInput 是 stub** — input.Tools 和 input.State 被忽略 | 安全性, 可维护性 |
| P1-2 | `agui/converter.go:557` | **SystemPrompt 通过 Capabilities GET 暴露** — 任意请求者可获取完整 System Prompt | 安全性 |
| P1-3 | `agui/converter.go:389-395` | **Thinking delta 未发 THINKING_TEXT_MESSAGE_END** — 对象类型存在但从未发射 | 正确性 |
| P1-4 | `a2ui/binding_agentcore.go:16` | **envelopeToMap 错误静默忽略** — `m, _ := envelopeToMap(env)` | 集成正确性 |
| P1-5 | `a2ui/binding_a2a.go:43-46` | **无法区分"非 A2UI"与"A2UI 格式错误"** — 两者返回相同信号 | 集成正确性 |
| P1-6 | `agui/client/src/` | **TypeScript 客户端无测试** — SSE 解析和事件分发无覆盖 | 测试质量 |
| P1-7 | `agui/` 全局 | **AGUI 测试覆盖率仅 77.7%** — 多个关键函数覆盖率 < 80% | 测试质量 |

### P2（12 项）— 建议在后续 PR 修复

| # | 文件 | 问题 | 维度 |
|---|------|------|------|
| P2-1 | `agui/converter.go:166-327` | Convert 162 行 switch，32 分支重复 | 可维护性 |
| P2-2 | `agui/handler.go:199-221` | threadCfgProviderFromConfig 本地鸭子类型接口 | 可维护性 |
| P2-3 | `agui/client/src/types.ts` | TypeScript/Go 类型手动同步无自动化 | 可维护性 |
| P2-4 | `agui/converter.go:318-326` | Convert default 分支暴露内部事件体 | 正确性, 安全性 |
| P2-5 | `agui/converter.go:420-422` | convertToolCallEnd err 覆盖 result | 错误处理 |
| P2-6 | `agui/handler.go:225-228` | writeSSE marshal 失败发 data:{} 歧义 | 错误处理 |
| P2-7 | `agui/converter.go:17-19` | Converter 原子操作 check-then-act 模式 | 并发安全 |
| P2-8 | `a2ui/validate.go:41` | ValidateEnvelope catalog=nil 跳过类型检查 | 正确性 |
| P2-9 | `a2ui/datamodel.go:138-141` | 数组删除设 nil 而非移除（按规范） | 正确性 |
| P2-10 | `agui/converter_test.go:11-23` | makeEvent JSON round-trip 间接测试 | 测试质量 |
| P2-11 | `a2ui/a2ui_test.go` | 断言多用 t.Fatalf 而非 t.Errorf | 测试质量 |
| P2-12 | `integration/` 无引用 | 缺少 A2UI/AGUI 集成测试 | 集成正确性 |

### P3（11 项）— 非阻塞建议

| # | 文件 | 问题 | 维度 |
|---|------|------|------|
| P3-1 | `agui/handler.go:104` | LoadAgent 错误暴露内部细节 | 安全性 |
| P3-2 | `agui/handler.go:72-74` | ThreadID 无校验直接做 store key | 安全性 |
| P3-3 | `agui/handler.go:227` | SSE eventType 无换行符校验 | 安全性 |
| P3-4 | `a2ui/component.go:47,52` | "id"/"component" 硬编码建议常量化 | 可维护性 |
| P3-5 | `a2ui/builder.go:99-205` | 20 组件构造函数手动维护 | 可维护性 |
| P3-6 | `a2ui/doc.go:32` | 包文档简短（已有 README 补充） | 可维护性 |
| P3-7 | `a2ui/surface.go:123-125` | ApplyUpdate 错误未包装 sentinel | 错误处理 |
| P3-8 | `a2ui/stream.go` | 未见性能热点 | 性能 |
| P3-9 | `agui/converter.go:389` | thinking msgID 每 delta 自增 | 性能 |
| P3-10 | `server/server.go:291` | `/agui/{path}` 通配符未用 | 集成正确性 |
| P3-11 | `a2ui/stream.go` | Encoder/Decoder 无并发安全声明 | 并发安全 |

---

## 各维度详细分析

### 维度 1: 正确性 — 7.5/10

核心问题：
- **P0-1/P0-2** 是两个阻塞性 bug，直接影响 SSE 客户端的正确行为
- **P1-1** thinking delta 协议合规性有疑，但影响有限（思考块最终能正常结束）
- A2UI 的 RFC 6901 实现、envelope 解析、StateStore 操作经 107 个测试验证，正确性高

### 维度 2: 安全性 — 6.5/10

关键风险：
- **P1-2** SystemPrompt 泄漏是最严重的安全问题
- **P1-3** callConfig stub 意味着前端传的 tools 配置被静默忽略
- 其他问题风险较低，但建议添加防御性编码

### 维度 3: 性能 — 7.5/10

评估：无显著瓶颈。SSE Mutex 是必需的并发保护，非性能缺陷。Agent 事件频率受人类交互节奏控制，无高吞吐场景。

### 维度 4: 可维护性 — 6.5/10

主要问题：
- **P2-1** Convert 的 32 分支 switch（162 行）是最大的维护负担。新增事件类型易遗漏分支
- **P2-2** 鸭子类型接口隐式依赖 agentcore 内部类型
- **P2-3** TS/Go 类型无同步机制，静默漂移风险

### 维度 5: 测试质量 — 8.0/10

两极分化：
- **A2UI**: 100% 覆盖率，107 个测试，边缘用例完善 ✅
- **AGUI**: 77.7% 覆盖率，`convertHandoffEnd`(0%) `convertCompactionEnd`(0%) `GetThreadConfig`(0%) 等多函数未覆盖 ❌
- TypeScript 客户端零测试覆盖

### 维度 6: 集成正确性 — 7.0/10

- **P1-4/P1-5** binding 层错误处理有缺陷
- 双 A2UI 处理路径经分析为 **设计如此**，不是 bug
- 缺少 `integration/` 目录下的跨模块集成测试

### 维度 7: 错误处理 — 6.5/10

- 2 个 P0 bug 涉及错误处理（Content-Type 不一致 + 不发送 RUN_ERROR）
- convertToolCallEnd 的 err 覆盖 result 需要修复
- 其余为 P3 级别的微调建议

### 维度 8: 并发安全 — 7.5/10

整体良好：
- `defer unregister()` 模式防止回调泄漏 ✅
- `sync.RWMutex` 保护 config 读写 ✅
- Converter 原子操作为防御性编程（当前实际单 goroutine 使用）
- SurfaceStore 明确声明非并发安全

---

## 关键改进建议

### 短期（1-2 天）
1. 修复 P0-1: LoadAgent 成功后再设 SSE headers，或错误走 SSE 事件而非 JSON
2. 修复 P0-2: agent.Run 失败时发送 RUN_ERROR 事件
3. 修复 P1-2: SystemPrompt 暴露 — 脱敏或添加配置开关

### 中期（1-2 周）
4. 修复 P1-1: 实现 callConfigFromInput 完整逻辑
5. 提升 AGUI 测试覆盖到 95%+（重点 Convert、handoffEnd、compactionEnd）
6. 修复 P1-3: 实现 THINKING_TEXT_MESSAGE_END 发射逻辑

### 长期（1 个月+）
7. 重构 P2-1: Convert 使用泛型 helper 消除值/指针分支重复
8. 添加 TS 客户端测试（P1-6）
9. 添加 A2UI/AGUI 集成测试到 `integration/` 目录
10. 引入 Go/TypeScript 类型同步机制（P2-3）

---

## 验证结果

| 检查项 | 结果 |
|--------|------|
| `go vet ./a2ui/... ./agui/...` | ✅ 无警告 |
| `go build ./a2ui/... ./agui/...` | ✅ 编译通过 |
| `go build ./example/a2ui-demo/...` | ✅ 编译通过 |
| `go test -race -count=1 ./a2ui/...` | ✅ 通过（竞态测试） |
| `go test -race -count=1 ./agui/...` | ✅ 通过（竞态测试） |
| `go test -coverprofile= ./a2ui/...` | ✅ 100.0% 覆盖率 |
| `go test -coverprofile= ./agui/...` | ✅ 通过，77.7% 覆盖率 |
| TS 编译（`tsc --noEmit`） | ✅ 通过 |
| 无 TODO/FIXME/HACK | ✅ 生产代码无标记 |
| 无 panic（生产代码） | ✅ 仅测试代码引用 |
| 无 goroutine（生产代码） | ✅ 仅测试代码使用 |
