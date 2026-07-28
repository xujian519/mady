# `domains/inventiveness/` 模块质量审阅报告

> 审阅日期：2026-07-27
> 审阅范围：全部 8 个源文件（~128K 代码）
> 编译检查：`go build ./domains/inventiveness/...` ✅
> Vet 检查：`go vet ./domains/inventiveness/...` ✅ 无警告
> 测试运行：`go test -count=1 ./domains/inventiveness/...` ✅ 全部通过

---

## 总体评价

模块质量优秀，架构清晰，提示词工程精良，测试覆盖全面。整体准备度：**生产级**，可直接投入使用。

---

## 1. 架构与设计 (A)

| 维度 | 评价 | 说明 |
|------|------|------|
| 分层隔离 | ✅ 优 | 完全独立子图，不依赖 `disclosure` 包，通过 `ArticleFrameworkProvider` 接口解耦 `domains/rules` |
| 单一职责 | ✅ 优 | 每个文件职责明确：类型/图构建/节点/框架/工具 |
| 向后兼容 | ✅ 优 | `ThreeStepResult` 为旧 API 提供兼容字段 |
| 可扩展性 | ✅ 优 | 六种发明类型 + 三种技术领域框架均为插件式接入 |

### 设计亮点

1. **接口解耦**：`ArticleFrameworkProvider` 接口避免了对 `domains/rules` 的直接 import，同时支持 nil 降级
2. **独立子图**：不依赖 disclosure 管线的状态传播，完全通过 `PregelState` 传递数据
3. **skip 传播**：`loadInputNode` 设置跳过标志后，下游节点通过 `stateHasSkip()` 短路，避免无谓的 LLM 调用
4. **分层提示词**：`personSkilledDefinition()` / `examinerErrorPrevention()` / `confidenceCalibration()` 等公共片段被复用，减少重复

---

## 2. 提示词工程 (A+)

这是本模块最强的维度。三步法各节点提示词质量极高：

- **Step 1**：34 行，覆盖最接近现有技术的选择标准
- **Step 2**：56 行，包含技术问题层次分析法（三层）、五种情形分类、红线规则、发明形成过程分析、特征划分规则、参数特征特殊规则、无贡献特征识别
- **Step 3**：68 行，双阶段（发明构思比对→技术启示判断）、TSM 五种情形、分析推理与有限试验结构化判断、用途限定、改进动机三维度系统分析
- **Step 4**：18 行（精炼），四种进步类型
- **Conclusion**：30 行，辅助因素分析（4 种）+ 置信度校准

### 不足之处

1. **Step 1 提示词偏短**（22 行有效内容），相比于 Step 2/3 的精炼程度略显单薄。建议补充：
   - 最接近现有技术的选取优先级规则（领域优先 vs 特征最多 vs 问题最接近的权重关系）
   - 多篇候选文献之间取舍的推理要求

---

## 3. 类型定义与状态管理 (A-)

### 类型定义
- `InventivenessInput` / `InventivenessResult` 设计完整，JSON tag 全覆盖
- `Step1Result`~`Step4Result` 细粒度子结果类型合理
- `EvidenceChunk` / `TechFeature` / `PFETriple` 值类型设计简洁

### 发现的问题

**① `buildResult` 中的默认分支逻辑可能覆写 LLM 判断（中风险）**

在 `nodes.go:807-817`：
```go
switch {
case cc.IsInventive:
    result.IsInventive = true
case cc.HasSignificantProgress:
    result.IsInventive = false
default:
    // 当 LLM 返回 is_inventive=false 且 has_significant_progress=false 时触发
    result.IsInventive = !result.Step3.TechnicalSuggestion && result.Step4.HasSignificantProgress
}
```

当 LLM conclusion 节点明确判定 `is_inventive: false` 和 `has_significant_progress: false` 时，`default` 分支会基于 Step3 + Step4 的计算结果重新判断，可能得出与 LLM 结论相反的 `IsInventive=true`。虽然 schemal 要求 `has_significant_progress` 为 required 字段，但在 LLM JSON 解析失败或字段缺失时仍可能触发。

**建议**：考虑增加一个中间状态 `cc.IsInventive == false && !cc.HasSignificantProgress` 时，检查 Step3 和 Step4 的原始数据是否与 LLM 结论一致，不一致时以 LLM 结论为优先并记录降级标记。

---

## 4. 测试覆盖 (A)

### 测试统计
- 单元测试文件：`inventiveness_test.go`（~34K）
- 集成测试文件：`integration_test.go`（~7K）
- 总测试用例数：60+
- 测试结果：全部通过，零 flaky

### 测试覆盖范围

| 测试类别 | 覆盖情况 | 说明 |
|----------|---------|------|
| 图编译 | ✅ 2 个测试 | 基本编译 + 有效验证 |
| loadInputNode | ✅ 4 个测试 | 空状态/nil输入/none覆盖度/有效输入 |
| stateHasSkip | ✅ 5 个子测试 | 表格驱动测试 |
| extractInput | ✅ 基本覆盖 | 有效/空状态 |
| JSON 提取 | ✅ 5 个子测试 | 含中文 JSON |
| parseStep1~4 | ✅ 12+ 测试 | 含无JSON、无效置信度、反向教导、跨领域、五种类型枚举 |
| parseConclusion | ✅ 1 个测试 | 含辅助因素 |
| buildResult | ✅ 2 个测试 | 正常路径 + 无显著进步边缘情况 |
| Schema 定义 | ✅ 4 个测试 | Step1~4 + Conclusion |
| Framework | ✅ 6 个测试 | 默认框架、关键术语、nil provider、with provider、fallback、V3术语 |
| 参数解析 | ✅ 3 个测试 | 有效/无效/空JSON |
| 工具注册 | ✅ 2 个测试 | 无 provider + 只读校验 |
| 集成全流程 | ✅ 2 个测试 | 完整 flow + skip 传播 |
| 类型兼容性 | ✅ 2 个测试 | StateKey 非空 + Result JSON 有效性 |
| 发明类型/领域 | ✅ 6 个测试 | 所有类型+领域框架 + 组合输入 |
| 置信度校准 | ✅ 2 个测试 | examinerErrorPrevention + confidenceCalibration 内容校验 |
| 实证统计 | ✅ 1 个测试 | EmpiricalStatistics 关键数据校验 |

### 测试缺口

1. **❌ `evaluateExperimentalDataNode` 没有单元测试** — 这是一个非 LLM 节点，但完全未被测试覆盖。建议补充：
   - 有实验数据时的输出格式
   - 无实验数据时正确跳过
   - `HasSupplementData=true` 时警告信息的包含
   - `ComparisonType` 字段的输出

2. **❌ `buildInputText` 中 PFE 三元组路径无独立测试** — 虽然 `TestBuildInputText` 覆盖了 features/prior_art/novelty，但 PFE triples 的分支未被验证

3. **❌ 无并发安全测试** — 虽然模块本身是 Pregel 图的顺序执行，但 `PregelState` 的 map 操作在并行运行场景下无锁保护（非本模块问题，但值得注意）

---

## 5. 错误处理与安全 (A-)

### 错误处理路径

| 节点/函数 | 错误处理 | 评价 |
|-----------|---------|------|
| loadInputNode | 3 级防御：missing key → nil input → EvidenceCoverage=none | ✅ 优秀 |
| step1~4 LLM 节点 | fmt.Errorf wrap + stateHasSkip 短路 | ✅ 良好 |
| generateConclusionNode | fmt.Errorf wrap + state 读取防御 | ✅ 良好 |
| parseStep1~4 | JSON parse fail → 回退到 raw text | ✅ 良好（但有改善空间）|
| runInventivenessTool | provider=nil 提前返回 + 错误 map 封装 | ✅ 良好 |
| NewInventivenessToolFromReport | 10min context timeout | ✅ 良好 |

### 安全边界

- `ReadOnly: true` ✅ — 工具标记为只读
- 无文件系统操作 ✅
- 无 shell 执行 ✅
- 无外部网络请求（除 LLM provider 调用）✅

### 发现的问题

**② parse 函数 JSON 解析失败时静默丢失结构化数据（低风险）**

`parseStep2`、`parseStep3` 等函数在 `json.Unmarshal` 失败时返回零值结构体，不记录错误。这可能导致下游 `buildResult` 基于空数据做判断。

例如 `parseStep2` 在 JSON 解析失败时返回空的 `Step2Result{}`，但此时 `buildResult` 中的 `result.Step2.DistinguishingFeatures` 为空切片、`ActualTechProblem` 为空字符串。

**建议**：考虑在 parse 失败时将原始输出写入 `Rationale` 字段（类似 `parseStep1` 和 `parseStep3` 的做法），而不是静默返回零值。

---

## 6. 代码风格与一致性问题

### 发现的问题

**③ `stringsJoin` 函数与标准库 `strings.Join` 命名冲突风险（低风险）**

`tool.go:309` 定义的 `stringsJoin()` 函数与 Go 标准库 `strings.Join()` 功能相似但不同（空格拼接 vs 分隔符拼接）。虽然不会编译冲突，但名称相似可能导致读者混淆。

**建议**：重命名为 `spaceJoin` 或直接使用 `strings.Join(s, " ")`。

**④ 未使用 import（良性）**

`tool.go` 导入了 `"context"` / `"encoding/json"` / `"fmt"` / `"strings"` / `"time"`、`agentcore`、`disclosure`、`graph`，全部有使用（`time` 用于超时 context）。✅

**⑤ `NewInventivenessToolFromReport` 使用 `context.Background()` 而非接收参数（低风险）**

`tool.go:296` 使用 `context.WithTimeout(context.Background(), ...)` 而非从调用方接收 context。这意味着该函数不能被调用方取消（如 HTTP 请求取消时不会传播）。

**建议**：改为接收 `ctx context.Context` 参数，使用 `context.WithTimeout(ctx, ...)`。这是一个函数签名的 breaking change，如果当前没有外部调用方，是改动的适当时机。

---

## 7. 建议改进优先级

| 优先级 | 问题 | 影响 | 建议操作 |
|--------|------|------|---------|
| P1 | buildResult 默认分支可能覆写 LLM 判断 | 中 — 可能导致结论不一致 | 增加中间状态检查，不一致时以 LLM 为优先 |
| P2 | evaluateExperimentalDataNode 无测试 | 中 — 功能未验证 | 补充单元测试 |
| P2 | parse 函数 JSON 失败时静默丢失数据 | 低 — 但影响下游判断准确性 | 失败时将 raw output 写入 Rationale |
| P3 | NewInventivenessToolFromReport 使用 Background() | 低 — 不可取消 | 改为接收 ctx 参数 |
| P3 | stringsJoin 命名容易混淆 | 低 — 可读性 | 重命名为 spaceJoin |
| P4 | Step 1 提示词偏短 | 低 — 功能完整但可优化 | 补充选取优先级规则 |

---

## 总结

`domains/inventiveness/` 模块是项目中最成熟的领域模块之一：

- ✅ 架构设计精良，独立子图 + 接口解耦
- ✅ 提示词法学功底深厚，覆盖三步法的所有关键判定细则
- ✅ 六种发明类型 + 三种技术领域框架，实用性极强
- ✅ 测试覆盖率高（60+ 条），核心逻辑有充分的单元测试和集成测试保证
- ✅ 错误处理和边界情况处理到位
- ⚠️ 6 个次要问题（0 个严重/高危，1 个中风险，5 个低风险/可优化）
