# Phase 3 审阅：R10 编排系统 + R11 任务管理 — 2026-07-25

> Phase 3 子审阅｜依据：`Mady 全面审阅计划 v1.0` ｜执行者：AI（Grok）｜Human Owner：[NEEDS CLARIFICATION]
> 审阅窗口：2026-07-24 新增代码（首次审）

## 摘要（3 条最关键发现）

1. **🔴 [H] R10 编排器完全缺失递归深度限防——重蹈 handoff #P1 覆辙**。`OrchestrationExecutor.Run` 调用 `agent.InvokeTool` 时既不递增 `WithDepth` 也不校验 `DepthFromContext`，而同仓库的 `handoff.go`、`task_tool.go`、`orchestrate.go` 都已用 `DefaultMaxDelegationDepth=8` 防护。一旦某个 manifest 的步骤 `ToolName` 设为 `run_orchestration`（自调用或 A→B→A 交叉），即无限递归→栈溢出。AGENTS.md 明确要求"编排系统必须吸取教训加深度限防"，此条直接违反。
2. **🟠 [M] 文档/实现严重不符：声称的"YAML 编译器"并不存在**。`orchestration.go` 注释与 AI_CHANGELOG 均宣称"YAML compiler lives in domains/orchestration_bridge.go"，但实际是**纯硬编码 Go switch**，无任何 `yaml.Unmarshal`。真正的 YAML 编排加载器在 `domains/rules/loader.go:140-150`，与 `agentcore.OrchestrationManifest` **互不连通**——存在两套并行且未衔接的"orchestration"概念。
3. **🟠 [M] R10 零递归/嵌套边界测试**。`orchestration_test.go` 仅覆盖顺序/条件/可选/中断，**完全没有**深度或自递归测试；而同包 `orchestrate_test.go` 却有现成的深度测试范式。

> **亮点**：R11 任务管理（tasklist）质量高——原子写设计正确（temp+rename）、循环依赖检测完整、UpdateFunc 原子读改写、深度限防到位、ReadOnly 标注全对、诚实标注批次内限制。**可直接合入**。

## 1. 审阅范围

### R10 编排系统（AgentToolchain）

| 文件 | 行数 | 说明 |
|---|---|---|
| `agentcore/orchestration.go` | ~222 | Manifest/Step/State/Result 数据模型 |
| `agentcore/orchestration_executor.go` | ~143 | 执行器（顺序步、条件、Optional、Interrupt） |
| `agentcore/orchestration_test.go` | ~290 | 7 个测试（**无深度测试**） |
| `domains/orchestration_bridge.go` | ~330 | case_type→manifest 硬编码 switch |
| `domains/orchestration_tools.go` | ~340 | `run_orchestration` 工具 + stateMappers |

> 另有 `agentcore/orchestrate.go`（~155 行，含 `DefaultMaxDelegationDepth` + `DepthFromContext`/`WithDepth`）与 `orchestrate_test.go`（含深度测试）属于**共享深度基础设施**，R10 编排器**未复用**。

### R11 任务管理（tasklist）

| 文件 | 行数 | 说明 |
|---|---|---|
| `agentcore/task_types.go` | ~108 | Task/TaskStatus/TaskPriority + Clone 深拷贝 |
| `agentcore/task_tool.go` | ~155 | TaskTool 委派（**含 TaskToolWithDepth 深度限防 ✓**） |
| `agentcore/tasklist/store.go` | ~140 | Store 接口 + MemoryStore |
| `agentcore/tasklist/filestore.go` | ~200 | 文件持久化（temp+rename 原子写） |
| `agentcore/tasklist/extension.go` | ~85 | Extension/ToolProvider/EventSnapshotProvider |
| `agentcore/tasklist/tool_create.go` | ~120 | task_create（非 ReadOnly） |
| `agentcore/tasklist/tool_get.go` | ~95 | task_get（ReadOnly ✓） |
| `agentcore/tasklist/tool_update.go` | ~260 | task_update + 循环依赖检测 |
| `agentcore/tasklist/tool_list.go` | ~100 | task_list（ReadOnly ✓） |
| 测试：store/extension/tools_test.go | 47 测试函数 | 循环依赖/self-block 齐全 |

## 2. 审阅维度执行情况（5 Lens）

| 维度 | R10 编排 | R11 任务管理 |
|---|---|---|
| Lens-1 Go 编码 | ✅ `%w` 包装；无 panic；MessageBus 用 recover 防 closed-channel（优秀） | ✅ `%w`；Clone 深拷贝正确；UpdateFunc 在克隆上 mutate；无 panic |
| Lens-2 架构/契约 | 🔴 **无深度限防**；🟠 YAML 编译器不存在；分层正确 | ✅ 分层正确；Extension 三接口到位；Store 接口合理；原子写正确 |
| Lens-3 安全红线 | ✅ run_orchestration 未标 ReadOnly（正确，有副作用） | ✅ ReadOnly 标注全对；🟡 FileStore 无 id 防御纵深校验（工具层已挡） |
| Lens-4 测试/门禁 | 🟠 **无递归/嵌套测试**；7 功能测试 | ✅ 循环依赖/self-block 齐全；🟡 无崩溃恢复/并发写测试 |
| Lens-5 核心理念 | ✅ 无 TODO；单文件 <350 行；🟠 编排抽象偏重但未接 YAML | ✅ 无 TODO；单文件 <260 行；抽象克制；诚实标注限制 |

## 3. 发现清单

| ID | 风险等级 | 类别 | 证据(文件:行) | 规范条款 | 建议 |
|----|---------|------|--------------|---------|------|
| **F1** | **H** | Lens-2 递归深度 | `orchestration_executor.go:83` InvokeTool 未用 WithDepth 递增；`agent.go:361-371` InvokeTool 无深度逻辑。对比 `handoff.go:119-121`/`task_tool.go:118-121`/`orchestrate.go:122-131` 均有 `depth >= maxDepth → ErrDepthExceeded` | AGENTS.md"编排系统必须吸取教训加深度限防"；handoff 敏感路径 | Run 入口校验 DepthFromContext，每个 InvokeTool 前 WithDepth(ctx, depth+1)；manifest 编译期拒绝 `step.ToolName == RunOrchestrationToolName` 自引用。复用 DefaultMaxDelegationDepth/ErrDepthExceeded |
| **F2** | **M** | Lens-5 文档/实现不符 | `orchestration.go:8-11` 注释"YAML compiler lives in domains/orchestration_bridge.go"；实际 `orchestration_bridge.go:31-46` 是纯 switch，无 yaml.Unmarshal。真正 YAML 加载在 `domains/rules/loader.go:140-150` 加载 rules.Orchestration，与 agentcore.OrchestrationManifest 互不连通 | AGENTS.md"变更即记录"准确性；"去繁就简" | 二选一：(a) 真实现 YAML→Manifest 编译器并桥接 rules.Orchestrations；(b) 修正注释/CHANGELOG，明确为"硬编码 manifest 注册表"，删除"YAML compiler"措辞 |
| **F3** | **M** | Lens-4 测试 | `orchestration_test.go` 7 测试无 depth/nesting/self-recursion；同包 `orchestrate_test.go:98-137` 已有 TestRunSequentialAgentsWithDepth_Exceeds 范式 | Karpathy"先写测试再修复" | 随 F1 修复补 3 测试：(1) 自递归 manifest 被拒；(2) A→B→A 交叉递归被 depth 截断；(3) depth+1 经 InvokeTool 正确传递 |
| F4 | L | Lens-4 持久化测试 | `filestore.go:171-182` temp+rename 原子写设计正确；`.tmp` 残留被 List 过滤；但无崩溃恢复/并发写测试 | 完成度约束 | 补：(1) 写入中途留 .tmp 后重启能正常 List；(2) N goroutine 并发 NextID+Create 后 ID 无重复 |
| F5 | L | Lens-1 错误处理 | `tool_list.go:49` `_ = json.Unmarshal(args, &p)` 忽略错误（注释"参数可为空"）；只读工具实际危害极低 | Karpathy"禁 _ = 忽略" | 改为显式 `if err := ...; err != nil { return nil, fmt.Errorf("参数无效: %w", err) }`，与 tool_get.go:48/tool_update.go:84 一致 |
| F6 | L | Lens-3 路径穿越纵深 | `filestore.go:71-78` Create 仅查 ID==""；Get/Update/Delete/taskPath 对 id 无格式校验；当前调用方工具层用 isValidTaskID（纯数字）拦截，今日无穿越路径 | AGENTS.md tools/path.go 沙箱边界 | FileStore 各方法入口加 isValidTaskID（或 filepath.Base(id)==id）校验；Store 接口注释固化"id 必须纯数字"契约 |

## 4. 已验证合规项

- ✅ **tasklist 原子写设计正确**：temp+rename（filestore.go:171-182），.tmp 残留被 List 过滤，loadNextID 健壮回退
- ✅ **tasklist 循环依赖检测完整**：hasCyclicDependency+canReach+visited map；self-block 单测；诚实标注"批次内新增边不检测"限制
- ✅ **tasklist UpdateFunc 原子读-改-写**：在 Clone 上 mutate，mutate error 不污染存储（有中止测试）
- ✅ **TaskTool 深度限防到位**（R11 的 task_tool.go）：TaskToolWithDepth 校验+WithDepth 递增，有专项测试
- ✅ **ReadOnly 标注全部正确**：task_get/task_list 标 ReadOnly；task_create/task_update/run_orchestration 不标（均有副作用）
- ✅ **分层隔离合规**：domains/orchestration_*.go 仅 import agentcore+pkg/util
- ✅ **MessageBus 并发 recover**（orchestrate.go:40-54）：防 cancel 与 send 间 TOCTOU 的 closed-channel panic，dropped 计数可观测——优秀
- ✅ **错误包装规范**：两模块普遍 `%w` 分层包装，无裸 panic
- ✅ **无 TODO/FIXME/HACK**：两模块 grep 全空
- ✅ **单文件行数克制**：最大 orchestration_tools.go ~340 行
- ✅ **Interrupt 语义自洽**：executor.go:94-101 Success=true+InterruptedStep 重载

## 5. 建议下一步

1. **[必修·F1+F3]** 在 OrchestrationExecutor.Run 加深度限防（复用 DepthFromContext/WithDepth/ErrDepthExceeded），manifest 注册期拒绝 run_orchestration 自引用；补 3 个递归边界测试。**本次审阅唯一阻断项**，直接对应 AGENTS.md handoff #P1 教训。
2. **[应修·F2]** 决定 R10 是否真做 YAML 编译器；若否，修正 orchestration.go 注释与 AI_CHANGELOG 措辞。
3. **[建议·F4]** 补 tasklist 崩溃恢复 + 并发 NextID 测试（设计无需改）。
4. **[建议·F5+F6]** tool_list.go 显式处理错误；FileStore 加 id 防御纵深校验。
5. **[门禁]** F1 修复后必须 `make verify`（含 -race）通过，并在 AI_CHANGELOG 追加"R10 编排深度限防补强"记录。

> **结论**：R11 任务管理质量高（设计克制、测试扎实、循环检测完整），可直接合入；**R10 编排系统需先修 F1（深度限防）方可合入**，否则构成与 handoff #P1 同类的栈溢出隐患——尤其团队已具备成熟的深度限防基础设施（orchestrate.go）却未在新编排器中复用。
