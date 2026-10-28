# 专利文书模板目录

本目录存放专利代理/律师场景的**正式文档交付物模板**。所有模板共享同一份文书品牌契约（`DOCS.md` + `tokens.css`），保证不同交付物在视觉上保持一致、专业、可打印。

## 目录结构

```
assets/templates/patent/
├── DOCS.md                    # 文书品牌契约（设计语言 prose）
├── tokens.css                 # 可覆盖的 CSS 语义变量表
├── manifest.json              # 机器可读设计系统清单（模板白名单 + 内部记录模板清单，渲染工具据此解析模板 id 与合规检查范围）
├── README.md                  # 本文件
└── <template-id>/             # 每个模板一个目录（共 13 个，清单见下；结构同构）
    ├── SKILL.md               # agent 工作流与输入/输出契约
    ├── assets/template.html   # 单文件 HTML 骨架
    ├── example.html           # 虚构填充示例（风格确认用）
    └── references/            # 版式惯例、撰写口径与自查清单
```

## 设计目标

- **证据优先**：每条关键结论都有来源类型与 pin-cite 定位。
- **层级清晰**：抬头 → 文号 → 元数据 → 章节 → 证据/引用日志 → 落款。
- **A4 打印优先**：屏幕与纸质输出一致。
- **可品牌化**：通过 `tokens.css` 与渲染调用的 `brand` / `brandPath` 覆盖。
- **anti-AI-slop**：固定结构、强制引用、置信度徽章，避免空洞长文。

## 使用方式

### 1. 受控草稿渲染

agent 读取对应模板目录的 `SKILL.md` 与 `references/`，按其中「输入要求」准备受控草稿（`draft`：`fields` 与 `sections`/`rows`），然后调用 `render_patent_document`（`template` 传模板 id）渲染 HTML/PDF：模板解析、品牌注入、段落编号与 PDF 打印都由渲染管线完成——不再手工复制或填充 `assets/template.html`。交付前置门禁要求本会话已成功跑过 `rule_check` 与 `law_verify`。

### 2. 模板切换

| 任务 | 使用模板 |
|---|---|
| 评估授权前景 | `patentability-opinion` |
| 出具检索报告 | `search-report` |
| 出具表格式检索报告 | `search-report-form` |
| 答复审查意见 | `oa-response` |
| 答复补正通知 | `rectification-response` |
| 提出复审请求 | `re-examination-request` |
| 撰写申请文件 | `claims-spec` |
| 发起/分析无效 | `invalidation-opinion` |
| 答复无效宣告请求 | `invalidation-defense` |
| 侵权比对意见 | `infringement-opinion` |
| 诉讼文书 | `litigation-pleading` |
| 转制 / 模拟专利权评价报告 | `right-evaluation-report` |

### 3. 品牌覆盖

渲染调用可传 `brand`（内联覆盖，键映射 `--sati-doc-*` CSS 变量）或 `brandPath`（指向 `theme.json`，取其 `documents.patent` 命名空间）；品牌变更**不改动模板 HTML**。可覆盖的变量见 `tokens.css`。

## 模板清单

| 模板 | 状态 | 场景 |
|------|------|------|
| `patentability-opinion` | ✅ 已落地 | 可专利性分析意见书 / 授权前景分析 |
| `search-report` | ✅ 已落地 | 专利检索报告 |
| `search-report-form` | ✅ 已落地 | 表格式专利检索报告（复刻《表格 220701》检索记录版式） |
| `oa-response` | ✅ 已落地 | 审查意见答复 / 意见陈述书（正式出件按 100012/100014 表格，见 `oa-response/references/conventions.md` §8） |
| `rectification-response` | ✅ 已落地 | 补正书答复 |
| `re-examination-request` | ✅ 已落地 | 复审请求书 |
| `claims-spec` | ✅ 已落地 | 权利要求书 / 说明书（含内部审稿版与正式提交版切换说明） |
| `invalidation-opinion` | ✅ 已落地 | 无效宣告请求书（对外件：请求人提交，不含内部评估与引用核验记录） |
| `invalidation-analysis` | ✅ 已落地 | 内部无效分析意见（内部件：可含引用核验记录、逐权项成功概率分档与缺口汇总；清单 `internalRecords` 列出，渲染器对其只做章节编号检查） |
| `invalidation-defense` | ✅ 已落地 | 无效宣告程序意见陈述书（对外件：**被请求人（专利权人）侧**的答辩，含程序性意见、质证意见、权利要求书修改与逐项答辩；正式出件按 100902/100014 表格，见 `invalidation-defense/references/conventions.md` §10） |
| `infringement-opinion` | ✅ 已落地 | 侵权比对意见 |
| `litigation-pleading` | ✅ 已落地 | 诉讼文书 |
| `right-evaluation-report` | ✅ 已落地 | 实用新型/外观设计专利权评价报告（复刻《表格 220701》版式） |

共 13 个模板，与 `manifest.json` 的 `templates` 列表一致。`search-report-form` 与 `right-evaluation-report` 是「表格式（复刻官方/检索记录版式）」家族：刻意使用黑白表格线、不套用 `tokens.css` 品牌色，以贴近官方原件外观；与品牌风的同族模板（如 `search-report`）是两种版式，按交付场合选用。无效宣告域按角色分三个产物：`invalidation-opinion` 是请求人侧的对外件，`invalidation-analysis` 是内部件，`invalidation-defense` 是被请求人（专利权人）侧的对外件。内部评估、成功概率分档、引用核验记录与缺口汇总只写入内部件。

## 相关

- `packages/patent/patent-document/README.md` — 受控草稿模型与渲染工具（`render_patent_document`）
- `docs/subsystems/patent.md` — 专利子系统服务面与交付通道
- 各模板目录内的 `SKILL.md` / `references/` — 模板级输入契约与撰写口径
