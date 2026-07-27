# A2UI / AGUI 审阅发现 — 维度 3-4（性能 + 可维护性）

## Dimension 3: Performance（性能）

### P3-1: SSE 写入锁持有时间
- **文件**: `agui/handler.go:128-137`
- **评估**: `sync.Mutex` 保护两个并发的 event callback（OnAll + On(EventTurnEnd)）对 `http.ResponseWriter` 的写入。锁持有期间包含 `Flush()` 调用。
- **修正计划**: 锁是必需的（两个回调可能并发触发）。Agent 事件频率受人类交互节奏限制，锁争用非瓶颈。**不下调级别，标记为 P3 观察点**。

### P3-2: Thinking delta msgID 自增
- **文件**: `agui/converter.go:389`
- **问题**: 每次后续 thinking delta 调用 `c.msgSeq.Add(1)` 分配新 msgID。
- **影响**: 开销极微（原子 Int64 自增），但 msgID 在前端可能无实际用途。
- **建议**: 非阻塞，仅作观察。

### P3-3: applyTokens map 分配
- **文件**: `a2ui/datamodel.go:111`
- **评估**: 中间节点不存在时创建新 `map[string]any{}`。数据模型操作频率低（每次 updateDataModel 信封），分配开销可忽略。

### P3-4: JSON Encoder buffer
- **文件**: `a2ui/stream.go:16`
- **评估**: `json.Encoder` 默认 buffer 512 字节。大组件列表（100+）可能多次 buffer 扩容。可添加 `SetEscapeHTML(false)` 微优化。
- **影响**: 极小，仅影响 JSONL 编码场景。

---

## Dimension 4: Maintainability（可维护性）

### P2-5: Convert 类型 switch 32 分支重复
- **文件**: `agui/converter.go:166-327`（162 行）
- **问题**: 16 种事件类型各需值+指针两个分支，至少 12 对完全重复。新增事件类型容易遗漏某一分支。
- **代码量**: 162 行，~100 行纯重复逻辑。
- **重构建议**: 使用泛型 helper（Go 1.26+）或事件包装器统一值/指针分支。

### P1(复用): callConfigFromInput stub
- **文件**: `agui/handler.go:192-197`
- **问题**: 已在 P1-3 记录。从可维护性角度，stub 代码增加了理解负担，新开发者可能认为功能已实现。
- **建议**: 实现或删除。

### P2-6: threadCfgProviderFromConfig 鸭子类型
- **文件**: `agui/handler.go:199-221`
- **问题**: 本地定义了与 `agentcore.ThreadConfigProvider` 独立的 `provider` 接口。若 agentcore 的接口签名变更，此处隐式断链且无编译错误。
- **建议**: 直接引用 `agentcore.ThreadConfigProvider` 接口而非本地重复定义。

### P2-7: TypeScript/Go 类型无同步机制
- **文件**: `agui/client/src/types.ts`
- **问题**: 文件头部注释"与 Go agui/types.go 保持同步"，但无自动化校验。Go 类型新增字段或变更时，TS 类型可能不同步。
- **影响**: 前端在运行时才发现字段缺失。
- **建议**: 添加 CI 检查或使用 JSON Schema / OpenAPI 生成 TS 类型。

### P3-5: "id"/"component" JSON 键未常量化
- **文件**: `a2ui/component.go:47,52`
- **影响**: 极小。若协议规范变更 JSON 键名，需手动搜索替换。定义为常量更易维护。

### P3-6: Builder 组件构造函数手动维护
- **文件**: `a2ui/builder.go:99-205`
- **影响**: 从 A2UI v0.9.1 升级到 v1.0 时 20+ 构造函数需同步更新。建议用代码生成。

### P3-7: a2ui/doc.go 包文档简短
- **文件**: `a2ui/doc.go:32` 行（对比 AGUI 协议文档 867 行）
- **影响**: godoc 渲染时的包说明较简洁，但已有 README.md（165 行）补充。

### ✅ 已验证: Convert 值/指针分支覆盖率
- 测试已覆盖大多数事件类型的值+指针路径，但 `convertHandoffEnd` (0.0%) 和 `convertCompactionEnd` (0.0%) 完全无测试。将在维度 5 详细记录。
