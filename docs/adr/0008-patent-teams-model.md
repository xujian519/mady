# ADR 0008：专利多智能体团队采用「新增 domains/teams 会话协作层」

- **状态**：Accepted（2026-10-28 批准）
- **日期**：2026-10-28
- **相关**：`docs/plans/dsh-domain-import-tasks.md` 的 W-1 / T-R-02；`packages/patent/patent-teams`（dsh）

## 背景

计划从 deepseek-harness 引入 `patent-teams`：队长领导的持久团队、可续聊子代理、依赖感知任务、
邮箱消息、事件驱动共享任务调度器。Mady 已有三块相邻能力，形态与语义各不相同：

| Mady 现有组件 | 实际职责 | 与团队模型的关系 |
|---|---|---|
| `agentcore/worker` | 图节点内的确定性执行器（`Definition` / `Catalog` / `Executor.ToPregelNode` / `Monitor`） | 单节点执行原语，不管理成员生命周期 |
| `agentcore/tasklist` | 任务存储与工具（`Task` 含 `Blocks` / `BlockedBy` / `Owner` / `Metadata`，`FileStore` 持久化） | 已具备依赖 DAG 与认领语义，可直接复用 |
| `domains/provisions` | 按 manifest 注册 handoff 子代理（一次性委派，返回 `HandoffResult`） | 同步委派，不是可续聊成员 |
| `agentcore.New` + `Agent.Resume` | 多 Agent 实例；`Resume` 仅用于**中断后**从工具结果处继续 | 跨轮唤醒成员需额外实现 |
| `session.AgentStore.Load` | 加载 Agent 状态快照 | 跨会话恢复的基础 |

dsh `patent-teams` 的四个关键差异：队长-成员的可续聊子代理、JSONL 邮箱（`O_APPEND` + 投递租约）、
事件驱动的共享任务调度（idle/running 边界触发，不轮询）、团队锁与成员生命周期（spawn/interrupt/retire）。

## 决策

**新增 `domains/teams` 作为会话级协作层，复用现有存储与执行原语，不改造 `agentcore/worker`。**

具体边界：

- 复用：`agentcore/tasklist` 的 `Task` 存储与依赖语义（`BlockedBy` 已满足「依赖感知任务」）；
  `session.AgentStore` 的状态快照；`agentcore.New` 的多实例能力；`worker.Monitor` 的执行记录。
- 新增：邮箱（每成员一个 JSONL，`O_APPEND` 追加、投递租约与重试）、事件驱动调度器
  （成员进入 idle 或任务图变更时尝试原子认领）、团队锁、成员生命周期、团队状态视图。
- 不改造 `agentcore/worker`：它是图节点执行器，把团队协作塞进去会混淆两个抽象层级。
- 团队状态目录置于案件工作区内（`.teams/<teamId>/`），事件写入会话日志，不新增 host 写 RPC。

## 后果

**正面**：
- 保持 `agentcore/worker` 的单一职责，团队能力可独立测试与卸载。
- 直接复用 `Task` 的依赖与 `Owner`，团队任务与既有任务列表共用一套数据模型，避免两套任务状态。

**负面 / 代价**：
- 「可续聊成员」在 Mady 没有现成等价物：`Agent.Resume` 只处理中断恢复，
  跨轮唤醒需基于状态快照重新装载上下文，可能引入上下文重复或丢失。
- 邮箱与调度器带来并发正确性负担（租约、重试、锁竞争），需要专门的竞态测试。
- 两套事件（团队事件 vs 会话事件）需要约定命名与顺序，否则回放困难。

**必须验证的前提**（实施前置，1 人日 spike）：
- [ ] 多个 `agentcore.Agent` 实例能否并发运行且互不干扰事件总线；
- [ ] 用 `AgentStore.Load` 恢复的 Agent 是否等价于「续聊成员」（上下文完整、工具状态一致）；
- [ ] 若上述不成立，降级为「串行 handoff + 邮箱队列」形态，并在 ADR 追加变更记录。

## 备选方案

| 方案 | 优点 | 缺点 | 结论 |
|---|---|---|---|
| A. 新增 `domains/teams`（本决策） | 职责清晰、可独立演进 | 需实现邮箱/调度/锁 | **采纳** |
| B. 扩展 `agentcore/worker` | 复用执行器与监控 | 混淆「节点执行」与「会话协作」，影响所有 worker 使用者 | 否决 |
| C. 直接移植 `patent-teams` 的 Cordis 服务形态 | 与上游一致 | Mady 无 Cordis，需引入服务容器；与 Extension 体系冲突 | 否决 |
| D. 不引入，维持 handoff + tasklist | 零成本 | 无法做异步多成员协作，长任务只能串行 | 保留为降级形态 |

## 实施要点（供 W5 使用）

1. 先跑前置 spike，把结论回写本 ADR 的「必须验证的前提」。
2. 邮箱与调度器先写并发测试（租约过期、重复投递、锁竞争），再接工具。
3. 团队事件命名遵循 `teams/<teamId>/<event>`，与会话日志的既有命名空间隔离。
