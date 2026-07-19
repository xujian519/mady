# agentcore 深度审阅报告

| 项 | 值 |
|---|---|
| 审阅日期 | 2026-07-20 |
| 审阅基线 | `bda2694`..`9f9846b` (HEAD) |
| 审阅范围 | agentcore/ 全量 182 文件（40 新增 + 29 修改 + 113 未变） |
| 审阅方法 | 8 阶段：基线验证→自动化扫描→历史回归→核心循环回归→三路并行精读（上下文引擎/扩展机制/支撑子系统）→安全并发专项核实→汇总 |
| 增量规模 | +10309 / -679 行（净增约占 agentcore 1/3） |
| 整体评级 | **良好（B+）** — 核心循环与并发原语扎实，扩展机制抽象清晰；主要风险集中在 planmode 安全绕过、插件/递归隔离边界、以及若干 UTF-8/资源处理缺陷 |

---

## 1. 执行摘要（TL;DR）

agentcore 在 `bda2694..HEAD` 周期经历了一次**大规模能力扩张**（净增 ~9.6K 行，占原体积约 1/3），新增了 doomloop（死循环熔断）、cache、concurrency、evaluate/benchmark+cli、tool_gen（工具 schema 生成）、reasoning_strategy（推理策略路由）、validate_config 等子系统。代码质量整体**良好**：

**亮点**
- `go build` / `go vet` / `golangci-lint` / `go test -race` **全绿**，lint 零 issue，无 TODO/FIXME 残留，工程整洁度极高
- 核心循环 `agent_run.go` 完成了一次高质量的净化重构（拆分 `runLoop`/`runInnerLoop`、提取 `failLoop`、configErr fail-fast），职责更清晰
- `hooks.go` 将 `time.After` 改为 `time.NewTimer + Stop`，修复了 timer 泄漏
- 错误包装链规范（`errors.Is/As` + `Unwrap` 完整），无该用 `%w` 却用 `%v` 的情况
- 中断状态机完整（Set/Get/Clear + 持久化 + Resume 清除），C5/C6 stream 泄漏已修复

**主要风险（需优先处理）**
- **2 Critical**：planmode 只读判定被 python/node/awk 等解释器绕过；pipeline 执行缺 panic 隔离
- **8 High**：handoff 无递归限深、filecheckpoint 沙箱校验缺失、doomloop detector 隐式串行假设、中文 UTF-8 截断、cli 评估路径损坏等
- **18 Medium / 25 Low**：集中在死代码、注释/行为不一致、边界处理与测试缺口

**历史回归结论**：P2-4（doc.go）、C5/C6（stream 泄漏）、manifest GuardrailLevel、P0-13（context.Background，从 ~25 处降至 8 处且多数合理）**全部回归通过**；P0-11 **部分残留**（NewHandoffError/NewGuardrailError 仍为零调用死代码）。

---

## 2. 范围与方法

### 2.1 范围
agentcore/ 子树全部 182 个 Go 源文件（558 非测试 + 283 测试口径下的 agentcore 子集），按模块域划分为：
- **核心循环**：agent_run / state / lifecycle / hooks / interrupt / steering / executor
- **上下文引擎与预算**：context_engine* / context_builder* / compaction* / budget / token / steering
- **扩展机制**：plugin / pipeline / atom / extension / skill_ext / reasoning_strategy / manifest / orchestrate / contract / handoff / tool_gen / validate_config
- **支撑子系统**：evaluate / evidence / permission / doomloop / planmode / filecheckpoint / tracing / cache / concurrency

### 2.2 方法（8 阶段）
| 阶段 | 内容 | 执行 |
|---|---|---|
| 0 | 基线验证（build/vet/lint/race/coverage）+ 增量识别 | 主会话 |
| 1 | 自动化扫描（禁用词/%w/io.Read/defer/context/time.After + 锁/goroutine/中断专项） | 主会话 |
| 2 | 历史回归（P0-11/P0-13/P2-4/manifest/C5-C6） | 主会话 |
| 3 | 核心循环变更回归（agent_run/state/hooks diff） | 主会话 |
| 4 | 上下文引擎与预算精读（22 文件） | task agent A |
| 5 | 扩展机制精读（20 文件） | task agent B |
| 6 | 支撑子系统精读（47 文件） | task agent C |
| 7 | 安全+并发专项核实（亲自验证全部 Critical/High） | 主会话 |
| 8 | 汇总报告 | 主会话 |

### 2.3 严重度定义
- **Critical**：安全漏洞 / 数据丢失 / 进程 crash / 核心隔离边界缺失
- **High**：功能错误 / 资源泄漏 / 并发 race / 安全敏感路径缺陷
- **Medium**：健壮性 / 边界处理缺失 / 设计权衡偏向风险
- **Low**：清洁度 / 文档 / 命名 / 测试缺口

---

## 3. 基线验证

| 检查 | 结果 |
|---|---|
| `go build ./...` | ✅ EXIT 0 |
| `go vet ./agentcore/...` | ✅ EXIT 0 |
| `golangci-lint run ./agentcore/...` | ✅ **0 issues** |
| `go test -race ./agentcore/...` | ✅ 全过，无 race |
| TODO/FIXME/HACK/XXX | ✅ 0 处 |
| `time.After`（循环泄漏风险） | ✅ 0 处（仅存者已改为 NewTimer+Stop） |
| `io.ReadAll` 无 Limit | ✅ 0 处 |
| TryLock/TryRLock | ✅ 0 处（无非阻塞抢锁反模式） |

### 覆盖率
| 包 | 覆盖率 | 备注 |
|---|---|---|
| agentcore (root) | 63.4% | 较历史 71% 下降，因 tool_gen(497)/plugin(179)/reasoning_strategy(263) 新代码稀释 |
| cache | 85.7% | ✅ |
| concurrency | 75.8% | ✅ |
| doomloop | 82.8% | ✅ |
| evaluate | 86.2% | ✅ |
| evaluate/benchmark | 50.0% | ⚠️ 偏低 |
| **evaluate/cli** | **0.0%** | ⚠️ cli.go 完全无测试 |
| evidence | 82.2% | ✅ |
| filecheckpoint | 51.0% | ⚠️ 偏低 |
| permission | 72.3% | ✅ |
| planmode | 74.1% | ✅ |
| tracing | 88.9% | ✅ |

---

## 4. 历史回归矩阵

| 历史项 | 来源 | 回归结论 | 证据 |
|---|---|---|---|
| **P0-11 死代码** | standards-review | ⚠️ **部分残留** | NewFatalError/NewRetryableError 已广泛使用（plugin/manifest/provider）；**NewHandoffError/NewGuardrailError 全仓零调用**（errors.go:147/169） |
| **P0-13 context.Background** | standards-review | ✅ **回归通过** | 从 ~25 处降至 8 处，且多为有意设计（handoff_context.go:107 有注释"不绑定请求生命周期"、event 必达消息、构造阶段注册） |
| **P2-4 无 doc.go** | standards-review | ✅ **已修复** | agentcore/doc.go 存在（983B），包文档完整 |
| **manifest GuardrailLevel 空串** | 安全敏感路径审计 | ✅ **回归通过** | manifest.go:103 `if m.GuardrailLevel != ""` 空串显式跳过，安全 |
| **C5/C6 stream goroutine 泄漏** | 历史回归 | ✅ **已修复** | stream.go stopWatcher 双处（:143/:197）正确，close+select Done |

---

## 5. 按严重度问题清单

### 🔴 Critical (2)

#### C1 — planmode 只读判定被解释器/awk 绕过 `[安全敏感]`
- **位置**：`agentcore/planmode/readonly.go:27-33,109`
- **问题**：`readOnlyCommands` 将 `go`/`cargo`/`node`/`python`/`python3`/`ruby` 标为 readOnly（true），`awk` 亦标 true。这些图灵完备解释器可执行任意代码：
  - `python -c "import os; os.remove('/etc/...')"`
  - `node -e "require('fs').writeFileSync(...)"`
  - `go run evil.go`
  - `awk 'BEGIN{print > "/etc/passwd"}'`（awk 内部重定向，`:109` 的 `strings.Contains(cmd, ">")` 检测不到）
- **后果**：plan mode 的"只读"语义被破坏，agent 可在只读阶段修改/删除文件。在专利/法律场景处理敏感文件时构成数据风险。
- **矛盾**：与文件首部注释 `:9` "Classification is conservative: if uncertain, treat as write" 直接冲突。
- **修复**：移除这些解释器或改为按 subcommand 白名单（如 `go test`/`go vet`/`go list` 放行，`go run`/`go build` 阻止）；对 awk/sed/printf 等具写文件能力的命令一律 fail-closed。

#### C2 — pipeline StageHandler 执行缺 panic 隔离
- **位置**：`agentcore/pipeline_executor.go:96`
- **问题**：`handler.Execute(ctx, state, e.provider)` 调用未包裹 `recover()`。StageHandler 实现包含第三方/插件代码与 LLM Agent 调用链，一旦 panic 会沿调用栈冒泡拖垮整个 doomloop 主循环。对比 `executor.go:320` 的工具并行执行路径**有** recover，pipeline 这条路径是隔离盲区。
- **后果**：这是扩展机制的核心隔离边界缺失。当前内置 handler 安全，但一旦引入第三方插件即 Critical。
- **修复**：在 stage 循环内为每次 `handler.Execute` 包 `defer func(){ recover() → 转 StageError }()`，保留 `InterruptStageError` 语义不被吞掉。

### 🟠 High (8)

#### H1 — handoff 缺递归限深，互相 delegate 致栈溢出 `[安全敏感]`
- **位置**：`agentcore/handoff.go:115-144`（executeDelegate）、`175-189`（handleTransfer）
- **问题**：两处调用子 Agent `sub.Run(ctx, ...)` 时**未传递 `WithDepth(ctx, depth+1)`**，也无 `DepthFromContext >= maxDepth` 检查。对比同模块 `task_tool.go:118-124` 是有 depth 防护的。若 A 配置 delegate 到 B、B delegate 到 A，子 Agent 同步 `Run` 持续消耗 goroutine 栈，绕过 `DefaultMaxDelegationDepth=8`，最终 stack overflow / OOM。
- **修复**：子调用前 `if DepthFromContext(ctx) >= DefaultMaxDelegationDepth { return ErrDepthExceeded }`，并用 `WithDepth(ctx, +1)` 调用。

#### H2 — filecheckpoint 沙箱校验缺失（isWithinRoot 死代码） `[安全敏感]`
- **位置**：`agentcore/filecheckpoint/store.go:73,131,154,198`
- **问题**：`isWithinRoot`（:198）定义完整但**全包仅测试调用**（store_test.go:205），生产代码 `SnapshotFile`/`Restore`/`restoreFile` 零路径校验。`Store.root` 字段（:34）沦为死代码。agent 可经 edit/write_file 指向 `../../../../etc/passwd`，Store 会忠实快照与回写，破坏 workspace 沙箱隔离。
- **修复**：在 `SnapshotFile` 入口和 `restoreFile` 写入前调用 `isWithinRoot(path, s.root)`，root 为空时 fail-closed。（注：上层 tools/path.go 或已隔离，但 defense-in-depth 缺失 + 死代码提供虚假安全感。）

#### H3 — doomloop detector 隐式串行假设（不持锁） `[安全敏感]`
- **位置**：`agentcore/doomloop/doomloop.go:204-220`
- **问题**：`AfterModelCall`/`AfterToolExecution` 中 `dl.totalToolCalls` 累加持 `dl.mu`（:200-202），但随后 `for _, d := range dl.detectors { d.RecordModelCall(mcc) }` 遍历与调用**不持锁**。detector 内部状态（`toolCallLoopDetector.last`、`textRepetitionDetector.lastLines`、`cycleDetector.history` 等可变字段）无锁保护。代码依赖"LifecycleHook 串行调用"的隐式假设，但该假设**未在 doc.go 或 AsHook 注释中文档化**。
- **后果**：单 Agent 串行调用下安全（race 测试通过佐证）；但若同一 DoomLoop 被多 Agent 共享（AsHook 返回共享 parent 的 hook），或主循环未来并发触发，会产生 data race 导致**漏报死循环**。
- **修复**：要么 detector 方法内部各自加锁，要么遍历期间持 `dl.mu`（OnSignal 回调仍需锁外执行，已正确）；并在 doc.go 显式声明"hook 必须串行"。

#### H4 — TieredEngine 按字节截断工具结果，中文 UTF-8 产生无效字符串
- **位置**：`agentcore/context_engine_tiered.go:172-176`
- **问题**：`snipToolResults` 用 `content[:min(e.snipHeadChars, len(content))]` 与 `content[len(content)-e.snipTailChars:]` 按字节切片，对多字节 UTF-8（中文）会在字符中间切断，产生**无效 UTF-8 字符串**送给 LLM。同仓库 `compaction.go:218-233` 的 `pruneOldToolResults` 已正确用 `[]rune` 处理，此处遗漏。
- **后果**：在 Mady 的专利/法律目标场景，工具结果几乎必然含中文 → 必然触发。provider JSON 编码失败报错，或模型收到乱码。
- **修复**：改用 `[]rune(content)` 切片后转回 `string`，与 `pruneOldToolResults` 一致；补中文测试用例。

#### H5 — evaluate/cli live 评估路径损坏且 0% 覆盖
- **位置**：`agentcore/evaluate/cli/cli.go:226-283`
- **问题**：(1) `runLive` 启动的 goroutine 无 `defer recover`，callProviderSimple panic 会导致进程 crash；(2) `callProviderSimple`（:266）用 `Complete(ctx, any) (any, error)` 类型擦除过度，`:282` `fmt.Sprintf("%v", resp)` 把响应结构体转为 Go 默认格式 `&{...}`，预测结果毫无意义；(3) 整个 cli 包覆盖率 **0.0%**，这些 bug 不会被任何测试捕获。
- **后果**：使用 `mady eval --live` 跑出的评估结果完全不可信。
- **修复**：补 `defer recover`（panic 时记录错误并 wg.Done）；修正 `callProviderSimple` 断言到含 `Content` 字段的具体类型；为 cli 包补充测试。

#### H6 — extension.Register 部分失败致资源泄漏
- **位置**：`agentcore/extension.go:70-74`
- **问题**：`Register` 循环中先调 `ext.Init`，若第 N 个扩展 Init 失败直接 `return err`，**前面已 Init 成功的扩展不会被 Dispose**，造成扩展资源（goroutine、文件句柄、订阅）泄漏。
- **修复**：失败路径逆序调用已成功 Init 的扩展的 `Dispose()`，或先全部 Init 成功再统一 push。

#### H7 — compaction summary 失败致原始消息永久丢失
- **位置**：`agentcore/compaction.go:414-430,496`
- **问题**：summary 生成失败（provider 报错）时，仍生成 fallback 文案并 `state.ReplaceMessages(compressed)`，被压缩区间 `[compressStart:compressEnd]` 的**原始消息永久丢失**（fallback 仅一句 "N message(s) were removed… could not be summarized"）。虽有 `summaryFailureCooldown=600s` 防重复失败，但**首次失败即不可逆丢数据**。
- **修复**：失败时保留原始消息不替换，仅记录错误并依赖冷却跳过后续压缩；或先压缩成功再提交。

#### H8 — planmode awk/sed 内部重定向绕过 `>` 检测 `[安全敏感]`
- **位置**：`agentcore/planmode/readonly.go:33,109`
- **问题**：`awk`（:33 标 true）的 `awk 'BEGIN{print > "/etc/passwd"}'` 输出重定向发生在 awk 内部，不需 shell `>` 操作符；`:109` 的 `strings.Contains(cmd, ">")` 检测不到。同理 `sed 'w file'`（sed 内部 write 命令）、`printf` 配合管道亦可构造写入。（`sed -i` 已显式标 false，但 sed 内部 write 绕过。）
- **修复**：对具写文件能力的命令（awk/sed/printf 等）一律 fail-closed。
- **注**：与 C1 同源（planmode 只读判定），合并修复。

### 🟡 Medium (18)

#### 上下文引擎与预算
- **M1** `agentcore/agent_persist.go:165` + `agent_run.go:492` — 压缩路径 `ReplaceMessages` 绕过 `BeforeMessagePersist`/`AfterMessagePersist` 钩子（注释自认 "bypasses this"）。压缩注入的 summary 对 guardrail/审计钩子不可见，形成审计盲区。建议增加专用 `BeforeCompactionPersist`/`AfterCompactionPersist` 钩子。[设计权衡，建议架构组确认]
- **M2** `agentcore/context_engine.go:162,184` + `compaction.go:268-279` — `CompressionBaseURL`/`CompressionAPIKey` 配置字段全局 grep 无读取使用，用户配置"独立压缩模型凭证"被**静默忽略**（实际用主 provider 凭证）。合规场景风险。建议：要么构建独立 Provider，要么移除字段并文档说明。
- **M3** `agentcore/budget.go:203-216` — `AfterModelCall` 仅事后累加 usage 不检查超限。预算为"事后记账"：本轮调用消耗后不熔断，直到**下一轮** BeforeModelCall 才触发。按量计费严格场景实际消耗可达 ~2× 上限。建议 AfterModelCall 累加后超限则异步触发 `fireExceed` 告警。
- **M4** `agentcore/context_builder_default.go:241` vs `token.go:10` — 同一流程内两套不一致的 token 估算：`EstimateTokens` 用字节/4，`truncateMessagesByTokens` 用 rune/4。中文场景偏差达 3 倍，致层间预算分配与全局压缩判定口径不一致。建议统一调用 `EstimateMessageTokens`。

#### 扩展机制
- **M5** `agentcore/reasoning_strategy.go:248-258` — 注释承诺"避免原地修改 so other observers see unmodified request"，但 `mcc.Request.Messages[i] = cp` 写回原 slice 底层数组（cp 只是 Message 值拷贝）。其它 BeforeModelCall 观察者读到的 system message 已被追加 hint，与注释承诺相反。建议：修正注释或真正克隆整条 Messages slice。
- **M6** `agentcore/tool_gen.go:153-183` — `schemaCache` 全局缓存不区分 lenient 模式。若首次以 lenient=true 调用先行缓存，后续 strict 调用拿到 lenient 版（无 Required）。调用顺序耦合的缓存是脆弱设计。建议：缓存键带 lenient bool，或始终缓存 strict 版返回前按需 relax。
- **M7** `agentcore/plugin_manager.go:48` — `input["_retriever"] = pm.retriever` 直接修改调用方传入的 input map。若调用方复用同一 map 跨多次调用，会累积污染。建议：注入移到 Run 内部 state copy 之后。
- **M8** `agentcore/handoff.go:217-224,243-298` `[安全敏感]` — HandoffTransfer 把 source 的**全部非 system 消息 + Tools/Extensions/Middleware/Lifecycle** 搬到 target。若 source 高权限、target 低权限，敏感上下文（密钥/凭证/当事人信息）从高护栏域流向低护栏域，违反最小权限。建议：增加 TransferableKeys allowlist 或走结构化 HandoffContext 抽取。
- **M9** `agentcore/tool_gen.go:466-471` — `tryCoerceValue` 对 int 字段先 ParseFloat，`"1.5"` 会 coerce 为 float64 写入，后续 Unmarshal 到 int 字段报错致整个工具调用失败。建议：ParseFloat 成功后检查 `f == math.Trunc(f)`，或 intKeys 分支只走 ParseInt。
- **M10** `agentcore/pipeline_executor.go:60-64` — `state := make(PipelineState, len(input))` 是浅拷贝。stage handler 修改引用类型值（slice/map）会回写调用方原始数据。建议：文档明确"浅拷贝语义"或对引用类型深拷贝。

#### 支撑子系统
- **M11** `agentcore/permission/permission.go:73-79` `[安全敏感]` — `if readOnly { return DecisionAllow }` 优先级高于 `p.Mode`。用户配置 `Policy{Mode: DecisionDeny}` 作默认拒绝时，read-only 工具仍被放行，与最小特权不一致。建议：文档明确优先级，或 Mode==Deny 时 readOnly 路径改 DecisionAsk。
- **M12** `agentcore/planmode/readonly.go:147-154` — `hasChainingOperator` 用 `strings.Contains(cmd, "|")`。`echo "a|b"`（引号内）或 `grep "a|b"`（正则）被误判为链式命令。建议：引号感知的 token 化。
- **M13** `agentcore/doomloop/doomloop.go:66,201,461` — `totalToolCalls` 字段持锁累加但**全包无人读取**（无 getter），与 `circuitBreaker.localCount` 重复计数。死代码增加维护混乱。建议：删除或暴露为 `TotalToolCalls()`。
- **M14** `agentcore/filecheckpoint/store.go:166-180` — `RestoreAndTrim` 非原子：先 Restore（释放锁做 IO）再 mu.Lock 修剪 `s.done`。Restore 释放锁期间并发 BeginTurn 会 append 新 checkpoint，被一并修剪掉，丢失活跃 checkpoint。建议：持锁期间先快照 target 与保留集，IO 完成后原子替换。
- **M15** `agentcore/evidence/extension.go:64-70` — `BeforeModelCall` 是死代码：注释写"Inject the ledger into context"，实际函数体只是 `_ = h.ext.agent` 啥也没注入。下游 `evidence.FromContext(ctx)` 永远拿不到 ledger。建议：实现 `ctx = WithLedger(...)` 或删除函数并修正注释。
- **M16** `agentcore/evaluate/benchmark/invalidation_decisions.go:26-30` — `init()` 解析 embed JSON 失败时仅 `fmt.Fprintf(os.Stderr,...)` 置 nil，不 panic。JSON 被损坏时 benchmark 集合悄悄少 100 条，CI gate 可能假绿。建议：失败时 `log.Fatalf` 或 AllCases 断言长度。
- **M17** `agentcore/evaluate/tool_accuracy.go:222-231` + `workflow_quality.go:170-177` — `parseToolCalls`/`parseWorkflowSteps` fallback 用 `strings.Index("[")`+`LastIndex("]")` 取最外层括号。LLM 输出含多个 `[...]` 块时取到含非 JSON 文本的子串，Unmarshal 失败默默得 0。建议：迭代尝试所有匹配或用健壮 JSON 提取器。
- **M18** `agentcore/evaluate/llm_judge.go:92-97` — `Samples > 1` 时**串行**发起 N 次完整 LLM 调用（默认 N=3×60s=3 分钟/题），全集可能数小时，且父 ctx 取消不短路。建议：有界并发或响应父 ctx 短路。

### 🟢 Low (25，精选)

#### 死代码（P0-11 残留 + 新增）
- **L1** `agentcore/errors.go:147,169` — `NewHandoffError`/`NewGuardrailError` 全仓零调用（P0-11 残留）。
- **L2** `agentcore/compaction.go:110-159` — `findCutPoint` 生产代码无调用者，仅测试（实际用 `findTailCutByTokens`）。
- **L3** `agentcore/evaluate/evaluator.go:105` — `report.TotalCases++` 被 :119 `len(cases)` 直接覆盖。
- **L4** `agentcore/evaluate/metrics.go:359-361` — `LengthScore.Compute` 中 `if max <= 0` 死代码（上文已归一化）。
- **L5** `agentcore/context_engine_truncate.go:55-62` — `TruncateEngine.ShouldCompact` 硬编码 `reserve := contextWindow/4`，忽略结构体字段 `thresholdPercent`（构造时存但从不读）。用户设 CompressionThreshold 对 truncate 引擎无效且无文档提示。

#### 文档/注释/行为不一致
- **L6** `agentcore/handoff.go:32-33` `[安全敏感]` — `HandoffConfig.AllowedSources` 字段注释暗示"空=不限制"，但 `isHandoffAllowed`（:309）是 default-deny（空=拒绝）。函数本身设计正确安全，但字段注释误导。建议改为"空=default-deny；含 * 或显式列出才放行"。
- **L7** `agentcore/validate_config.go:50-62` `[安全敏感]` — `validateHandoffs` 未校验 AllowedSources 内容（空字符串、`*` 与具体名混用、自循环）。建议启动期规范化校验。
- **L8** `agentcore/manifest.go:99-101` — domain 校验失败错误消息硬编码"有效值：chat/assistant/patent/legal"，但 validDomains 可动态扩展。建议从 map 动态拼接。
- **L9** `agentcore/evaluate/report.go:88-99` — `sortedMetricNames` 实际只去重不排序（map 遍历序不稳定）。建议改名 `collectMetricNames` 并 `sort.Strings`。

#### 健壮性/边界
- **L10** `agentcore/tool_gen.go:412-415` — `patchArgs` Unmarshal 失败时 `return raw, nil` 静默吞错，错误链丢失"输入非 object"信号。
- **L11** `agentcore/orchestrate.go:25-35` — `MessageBus.Publish` 订阅 channel 缓冲满时 `default` 分支静默丢弃，无告警/计数。关键协调消息可能静默丢失。
- **L12** `agentcore/doomloop/doomloop.go:531-546` — `IsDoomLoopFatal` 用 `strings.Contains(errStr, string(id))` 匹配，普通英文单词误判。建议结构化错误 + errors.As。
- **L13** `agentcore/permission/rule.go:120-124` — `extractMatchValue` fallback 依赖 map 遍历序（随机），规则判定不确定。
- **L14** `agentcore/budget.go:177` — `fireExceed` 无锁读 `c.budget`，同文件 `Budget()` 却加锁，API 契约不一致。
- **L15** `agentcore/context_engine.go:271-277` 等 — CompressorEngine/TieredEngine/ChunkedEngine 可变状态字段无 mutex（单 goroutine 安全，但诊断方法并发读理论 race）。建议注释说明或加锁。

#### 安全敏感（Low 但需记录）
- **L16** `agentcore/planmode/policy.go:48,55` `[安全敏感]` — `blockedTools`/`alwaysAllowed` 精确 map 查找（区分大小写），其它判断用 EqualFold。fail-closed 兜底使其安全，但风格不一致易误导维护者。
- **L17** `agentcore/filecheckpoint/extension.go:16-22` — `writerTools` 不含 `bash`，bash 改文件不被 checkpoint 捕获（回滚不一致）。[与 H2 同域]
- **L18** `agentcore/evidence/receipt.go:23-30` vs `filecheckpoint/extension.go:16-22` — `writerTools` 两处定义且不同步（receipt 含 execute_code）。建议抽到公共位置。
- **L19** `agentcore/filecheckpoint/extension.go:101-107` — `BeforeTurn` 传 `len(arc.Messages)` 作 msgIndex，应为 `len-1`（off-by-one）。

#### 并发/资源
- **L20** `agentcore/evaluate/llm_judge.go:119` + `reflection.go:234` — `context.WithTimeout(context.Background(), timeout)` 不传播父 ctx（已知 8 处之 2）。建议用 `context.WithoutCancel`（Go 1.21+）派生。
- **L21** `agentcore/extension.go:97-105,285-297` — `TransformContextProvider` 每扩展包一层闭包，inheritRuntime 再包，扩展多时深嵌套。建议扁平 `[]func`。
- **L22** `agentcore/atom.go:148-151` + `pipeline_handler.go:63-67` — `RegisterAtom`/`RegisterStageHandler` 注释明说"Duplicate names silently overwrite"，多 init 竞争时问题难定位。建议提供 OrError 变体或日志记录覆盖。
- **L23** `agentcore/evidence/ledger.go:40-43` — `Len()` 用 `Lock` 而非 `RLock`，纯读过度加锁。
- **L24** `agentcore/tracing/otel.go:93-94` — `case []attribute.KeyValue:` 分支不可达，Value 不会是 OTel 自身类型。
- **L25** `agentcore/evaluate/judge_metrics.go:80-147` — `GuardrailFalseNegativeRate`/`AdoptionRate` 有 Name() 但无 Compute()，不满足 Metric 接口却放在 evaluate 包易误用。

---

## 6. 按模块详细发现

### 6.1 核心循环（agent_run / state / lifecycle / hooks / executor / interrupt / steering）
**评级：优秀**。`bda2694..HEAD` 的 agent_run.go 重构质量高：
- 拆分 `runLoop`（外层 follow-up）与 `runInnerLoop`（内层 turn），职责分离清晰
- 提取 `failLoop()` 辅助消除重复的错误处理样板
- `Run()` 开头增加 `configErr` fail-fast
- 重复检测状态局部化到 `runInnerLoop`（注释明确"intentionally not shared across follow-up rounds"，合理设计）
- hooks.go 将 `time.After` 改为 `time.NewTimer + Stop`，修复 timer 泄漏
- state.go 补充 Status 文档

中断状态机（interrupt.go + state.go + agent_run.go:349/607-670）完整：触发→记录 reason→持久化 checkpoint→返回 ErrInterrupt→Resume 时 ClearInterruptReason。

### 6.2 上下文引擎与预算（22 文件）
**评级：良好**。核心并发原语（BudgetController.mu、messageQueue.mu、AgentState.mu）正确，错误链完整。主要问题：H4（中文 UTF-8）、M1-M4（审计盲区/配置静默忽略/事后记账/估算不一致）。

### 6.3 扩展机制（20 文件）
**评级：良好**。核心抽象（Plugin/Pipeline/Atom/ReasoningStrategy）清晰，并发锁规整，InterruptStageError 传播正确。主要风险：C2（pipeline 隔离）、H1（handoff 递归）、H6（extension 泄漏）、M5-M10。

### 6.4 支撑子系统（47 文件）
**评级：需改进**。问题最集中：
- **planmode**：C1/H8（只读绕过）— 最严重
- **evaluate/cli**：H5（损坏 + 0% 覆盖）
- **filecheckpoint**：H2（沙箱）+ 覆盖率 51%
- **doomloop**：H3（隐式串行）+ M13（死代码）
- **permission/evidence**：M11/M15
- **cache/concurrency/tracing**：优秀（仅 1 Low 各）

---

## 7. 修复路线图

### P0（立即，1-3 天）
1. **C1 + H8 planmode 绕过**：移除 python/node/go/ruby/cargo/awk 的 readOnly 标记，改 subcommand 白名单；awk/sed/printf fail-closed
2. **C2 pipeline 隔离**：为 `handler.Execute` 包 recover
3. **H1 handoff 递归限深**：executeDelegate/handleTransfer 加 WithDepth + 深度检查

### P1（本周）
4. **H2 filecheckpoint 沙箱**：启用 isWithinRoot 校验
5. **H3 doomloop 文档化/加锁**：明确串行假设或 detector 内加锁
6. **H4 中文 UTF-8**：snipToolResults 改 []rune
7. **H5 evaluate/cli 修复 + 补测试**：recover + 类型断言 + 覆盖率
8. **H6 extension.Dispose**：失败路径逆序 Dispose
9. **H7 compaction 不丢消息**：失败时保留原文

### P2（本月）
10. **M1-M4 上下文引擎**：压缩钩子、CompressionBaseURL 决策、预算事后告警、token 估算统一
11. **M5-M10 扩展机制**：reasoning 注释/实现一致、tool_gen 缓存键、plugin input 隔离、handoff transfer allowlist、coerce int、pipeline 深拷贝
12. **M11-M18 支撑子系统**：permission readOnly 优先级、planmode 引号感知、doomloop 死代码清理、filecheckpoint 原子性、evidence 死代码、benchmark init 校验、JSON 提取、llm_judge 并发

### P3（ opportunistically）
13. **L1-L25**：死代码清理、注释修正、测试补全（cli 0%→目标 60%+、filecheckpoint 51%→70%+、benchmark 50%→70%+）

---

## 8. 质量趋势（对比历史审阅）

| 维度 | Phase 3 (2026-07-14) | REVIEW_REPORT (2026-07-16) | 本次 (2026-07-20) | 趋势 |
|---|---|---|---|---|
| agentcore 评级 | 优秀 | —（16 Critical 全修复） | 良好（B+） | ➡️ 持平偏降（因大规模扩张引入新缺陷） |
| 覆盖率(root) | 71% | — | 63.4% | ⬇️ 新代码稀释 |
| Critical | 0（1 Low） | 16（全修复） | 2 | ⬆️ 新增（planmode/pipeline） |
| lint | — | — | 0 issues | ✅ 持续优秀 |
| race | — | — | 全过 | ✅ |
| TODO/死代码 | planner.go TODO | P0-11 待修 | TODO=0；P0-11 部分残留 | ➡️ TODO 清零，死代码新增 |

**解读**：本周期 agentcore 经历"扩张期"，新增 ~9.6K 行带来 doomloop/cache/concurrency/evaluate 扩展等能力，工程质量基线（lint/race/vet）保持极高水准，但**扩张速度快于防护补齐速度**——planmode 安全绕过、pipeline 隔离、handoff 递归等"扩展边界"问题集中暴露。建议下一周期聚焦"边界加固"而非继续扩张。

---

## 9. 附录：覆盖矩阵与文件清单

### 9.1 子目录审阅覆盖
| 子目录 | 文件数(非测试/测试) | Critical | High | Medium | Low | 评级 |
|---|---|---|---|---|---|---|
| agentcore (root) 核心循环 | ~15 | 0 | 0 | 0 | 0 | 优秀 |
| 上下文引擎与预算 | 11/11 | 0 | 1 | 4 | 4 | 良好 |
| 扩展机制 | 20 | 1 | 2 | 6 | 8 | 良好 |
| evaluate/ | 14/9 | 1 | 1 | 4 | 6 | 需改进 |
| evaluate/cli/ | 1/0 | 1 | 1 | 1 | 0 | 需改进 |
| evaluate/benchmark/ | 9/6 | 0 | 0 | 1 | 1 | 合格 |
| evidence/ | 7/5 | 0 | 0 | 0 | 2 | 优秀 |
| permission/ | 4/1 | 0 | 0 | 1 | 1 | 良好 |
| doomloop/ | 2/1 | 0 | 1 | 1 | 1 | 良好 |
| planmode/ | 4/1 | 1 | 2 | 1 | 1 | 需改进 |
| filecheckpoint/ | 4/1 | 0 | 2 | 1 | 1 | 需改进 |
| cache/ | 3/1 | 0 | 0 | 0 | 1 | 优秀 |
| concurrency/ | 2/1 | 0 | 0 | 0 | 1 | 优秀 |
| tracing/ | 2/1 | 0 | 0 | 0 | 1 | 优秀 |
| **合计** | **~89 核心+测试覆盖** | **2** | **8** | **18** | **25** | **良好** |

### 9.2 关键文件位置速查
- 核心循环：`agent_run.go`(710行)、`state.go`、`lifecycle.go`、`hooks.go`、`executor.go`、`interrupt.go`、`steering.go`
- 上下文：`context_engine*.go`(4)、`context_builder*.go`(2)、`compaction*.go`(2)、`budget.go`、`token.go`
- 扩展：`plugin*.go`、`pipeline_*.go`、`atom.go`、`extension.go`、`reasoning_strategy.go`、`manifest*.go`、`handoff*.go`、`tool_gen.go`(497新增)、`validate_config.go`
- 支撑：`evaluate/`(30)、`evidence/`(8)、`permission/`(4)、`doomloop/`(3)、`planmode/`(3)、`filecheckpoint/`(3)、`cache/`(3)、`concurrency/`(3)、`tracing/`(2)

---

*报告完。所有发现均引用具体 `file:line` 证据，Critical/High 经主会话亲自核实。基线 commit `bda2694`，HEAD `9f9846b`。*
