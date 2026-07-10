# Mady 工作流指南

Mady 内置三种可调用的专业分析工作流。

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
       → 关键词生成 → 新颖性初判 → 报告生成 → 人工复核
```

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
  "report_text": "..."
}
```

## 2. 专利新颖性分析

对发明描述进行新颖性和创造性分析，含确定性规则引擎校验。

### 触发方式

通过 Patent Agent 的 `analyze_patent_novelty` 工具触发。

### 分析流程

```
解析 → 检索 → 特征比对 → 规则引擎校验 → 输出报告
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
