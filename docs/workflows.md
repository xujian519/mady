# Mady 工作流指南

Mady 内置多种可调用的专业分析工作流和推理引擎。

## 1. 技术交底书分析 (Disclosure)

对专利技术交底书进行自动化分析，提取技术问题、特征、效果，生成结构化报告。

### 触发方式

**通过 Server API：**
```bash
# 提交分析
curl -X POST http://localhost:8080/v1/disclosure/analyze \
  -H "Content-Type: application/json" \
  -d '{"text": "一种智能灌溉装置，其特征在于..."}'

# 返回: {"task_id":"disclosure-20260711-120000","status":"pending"}

# 轮询结果
curl http://localhost:8080/v1/disclosure/analyze/disclosure-20260711-120000

# SSE 实时流
curl http://localhost:8080/v1/disclosure/analyze/disclosure-20260711-120000/stream
```

**通过 Patent Agent：**
Patent Agent 配备 `analyze_disclosure` 工具，在对话中自动触发。

### 分析流程

```
预处理 → 问题提取 / 特征提取 / 效果提取 (并行)
       → 合并 → 一致性校验 (最多 2 轮回退)
       → 关键词生成 → 新颖性初判 (LLM JSON Schema)
       → 报告生成 → review_gate (主动中断等待人工确认)
```

> **review_gate 行为（v0.3.0+）**：之前 review_gate 是 no-op（仅设 `_gate_ready` flag），
> 现已改为返回 `agentcore.InterruptError`（复用已有中断机制，Pregel NodeError 透传），
> 在报告生成后强制暂停管线，等待人工 Resume/确认。此变化影响所有 disclosure 调用。

### 输出结构

```json
{
  "document": { "title": "...", "sections": {...} },
  "extraction": {
    "problems": ["..."],
    "features": [{"description":"...", "category":"structure"}],
    "effects": ["..."],
    "pfe_triples": [{"problem":"...", "feature_ids":["..."], "effect":"..."}]
  },
  "consistency": { "pass": true, "issues": [], "overall_score": 0.85 },
  "search_keywords": ["..."],
  "report_text": "...",
  "novelty_assessment": { "status": "passed", "confidence": 0.75 }
}
```

## 2. 专利新颖性分析

对发明描述进行新颖性和创造性分析，含确定性规则引擎校验。

### 触发方式

通过 Patent Agent 的 `analyze_patent_novelty` 工具触发。

### 分析流程

```
解析 → 检索(三路 RRF + cross-encoder 重排) → 特征比对
     → 规则引擎校验 → 证据注射 → 输出报告
```

### 规则引擎

内置 6 条专利审查规则：
- 单独对比原则
- 三步法（创造性）
- 充分公开
- 权利要求分析
- 全面覆盖原则
- 等同原则

每条规则输出 pass / needs_revision / blocked 三个等级。

## 3. 法律案例比较

对案件事实进行法律分析，检索适用法条和相似判例，生成三段论推理报告。

### 触发方式

通过 Legal Agent 的 `compare_legal_cases` 工具触发。

### 分析流程

```
案件事实 → 法条识别 → 判例检索 → 比较分析 → 三段论推理 → 结论
```

### 推理引擎

基于 FactBlackboard + Syllogism 引擎：
- 大前提：适用法条和规则
- 小前提：案件事实
- 结论：法律判断（附带推理链）

## 4. 五步工作法推理 (Five-Step Workflow)

> 专利分析的标准推理框架，融合事实发现、规则获取、规划执行与检查。

### 触发方式

Patent Agent 的 `run_five_step_workflow` 工具，或通过 `analyze_patent_novelty` 内嵌触发。

### 流程

```
Stage① 发现事实 (Fact Blackboard)
  → Stage② 获取规则 (四路召回 + 确认阀)
    → KG (0.9 权威度)
    → Vector (FTS5, 0.7 权威度)
    → Skill (wiki patent-cards, 0.4 权威度)
    → Rules (确定性规则引擎, 0.95 权威度)
    → 确认阀: 人工确认规则后才进入下一阶段
      → 中断 → SaveCheckpoint → ResumeFromCheckpoint
  → Stage③ 规划 (ConfirmedRuleSet 约束 Plan)
  → Stage④ 执行 (工具编排 + evidence 注射)
  → Stage⑤ 检查 (RuleAssertionHook 对比事实/规则验证)
```

### 确认阀闭环

`reviewMode` 开启时，Stage② 返回规则后触发 `InterruptError`（携带 `checkpoint_id`），
SaveCheckpoint 保存 blackboard 到 `CheckpointStore`，用户 /approve 后
ResumeFromCheckpoint + SetConfirmedRules 续跑 Stage③-⑤。

## 5. 插件工作流 (Plugin System)

> 可组合的专利工作流原型，基于 `plugin.json` + `SKILL.md` 定义。

### 内置插件

| 插件 | 组件 | Pipeline Stages |
|------|------|----------------|
| novelty-analysis | `plugins/patent/novelty-analysis/` | 5 stages |
| infringement-check | `plugins/patent/infringement-check/` | 7 stages |
| oa-response | `plugins/patent/oa-response/` | 6 stages |

### Pipeline Atoms

每个 stage 引用注册表原子（`RegisterAtom`/`LookupAtom`），覆盖完整专利工作流：
- **search** — 现有技术检索
- **extract** — 特征提取
- **compare** — 特征比对
- **reasoning** — 推理判断
- **approval-gate** — 审批关卡

与现有 `tool` 字段向后兼容。Atom 为空时退回到传统 Tool 名匹配。

## 6. 文档模板库 (Doc Templates)

> Markdown + `{{variable}}` 语法的文档模板，支持按 category 分组使用。

| 类别 | 模板数 | 存放位置 |
|------|:------:|----------|
| Claims (权利要求) | 3 | `doc-templates/claims/` |
| Specification (说明书) | 4 | `doc-templates/specification/` |
| OA Response (答复) | 3 | `doc-templates/oa-response/` |
| Disclosure (交底书) | 2 | `doc-templates/disclosure/` |

通过 `go:embed` 编进二进制，用户可在 `$MADY_HOME/doc-templates/` 覆盖或新增。

## 8. 专利工作流编排 (AgentToolchain)

Mady 的 AgentToolchain 编排系统通过 `run_orchestration` 工具一次串联多个领域工具。

**编排清单：**
| 编排名称 | 说明 | 步骤 | 触发方式 |
|----------|------|:----:|----------|
| oa_response | OA答复全流程 | 8步（含条件分支） | run_orchestration("oa_response") |
| patent_drafting | 专利撰写全流程 | 5步 | run_orchestration("patent_drafting") |
| re_examination | 复审请求 | 4步 | run_orchestration("re_examination") |
| invalidation | 无效宣告 | 4步 | run_orchestration("invalidation") |

**oa_response 编排流程：**
```
parse_office_action (解析OA)
  → [条件] analyze_enablement (26.3判断)
  → [条件] analyze_inventiveness (创造性判断)
  → [条件] analyze_patent_novelty (新颖性分析)
  → get_article_framework (法条框架)
  → validate_amendment (A33校验)
  → draft_oa_response (起草答复书)
  → analyze_slop (套话检查)
```

条件分支根据 OA 驳回类型自动判断。Patent Agent System Prompt 优先调用编排工具，五步工作法降为降级方案。

## 9. 证据判断工作流

通过 `mady evidence` CLI 或 EvidenceDomainExtension 的 5 个工具触发。

| 工作流 | 触发方式 | 说明 |
|--------|----------|------|
| 证据三性判断 | judge_triple 工具 | 真实性/合法性/关联性三维度自动评判 |
| 举证责任查询 | check_burden 工具 | 举证责任分配规则查询 |
| 证明标准评估 | assess_standard 工具 | 各证明标准适用场景评估 |
| 证据冲突检测 | detect_conflict 工具 | 多份证据间的矛盾判定 |
| 类型化证据评价 | judge_type_specific 工具 | 按证据类型的专门评价规则 |

5 个工具已集成到无效宣告和侵权比对工作流中。
