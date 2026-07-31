# 前端 i18n（国际化）评估 — Mady Desktop

> 对应 `docs/plans/desktop-next-development-plan.md` W4-T9（缺口 P1-4，案例参考 `docs/mady-desktop-standards.md` §13.1）。
> 状态：**评估完成**（2026-07-31）。
> 对应验收标准：① 评估报告输出（本文档）✅；② 原型验证（至少一个关键页面可切换 zh-CN/en-US）→ 本期实施范围第 4 章。

---

## 0. 结论速览（TL;DR）

| 项 | 结论 |
|----|------|
| 现状 | 后端已有 `pkg/i18n`（zh-CN/en-US，点号键、zh-CN 兜底回退），但**引导（bootstrap）缺失**——全局目录从未被 `SetGlobal`/`LoadDir` 初始化，护栏文案运行时回退为 key 原文；前端**无任何 i18n 库**，约 **200+ 条**面向用户的硬编码中文文案集中在 `components/`（约 179 行非注释文案行） |
| 方案选型 | **react-i18next**（+ i18next），JSON 资源，插值用 `{{var}}`；不引入 extra 插件，贴合项目「克制」哲学 |
| 术语一致性 | **单一源 + 生成脚本**：以 `pkg/i18n/translations/` 的 YAML 为唯一术语源，`scripts/gen-frontend-i18n.mjs` 生成前端 `locales/{zh-CN,en-US}.json`，CI 做键集合一致性检查；双源人工同步 + 差异检查作为备选兜底 |
| 切换与默认语言 | 默认 **zh-CN**；切换入口放设置面板「外观」区；语言偏好持久化到前端 settings store（localStorage）并镜像写入后端 `~/.mady/desktop-settings.json` 的 `locale` 字段 |
| 本期范围 | 评估报告 + 设置面板单页原型（对应验收标准 2），P0 文案约 40-60 条；全量抽取放下一迭代 |
| 工作量 | 评估 0.5-1 人天（含在 W4-T9 排期）；实施 3-5 人天 |

> **✅ 产品决策（2026-07-31）**：前端 i18n **推迟到最终发布前实施**；届时**只做中文简体（zh-CN）单语言版**，不引入 react-i18next 运行时。
> 含义：
> - 发布前的「i18n 实施」降级为**文案抽取与集中管理**（把散落组件的硬编码中文收敛到单一 `locales/zh-CN.json` + 常量表），不构建 en-US 资源与切换入口；
> - 后端 `pkg/i18n` 的 SetGlobal 接线（1.1 节指出的 bootstrap 缺失）仍建议顺手修复，避免护栏文案回退为 key 原文；
> - 若未来确有多语言诉求，再按本报告 §2.2 方案引入 react-i18next（术语源单一化机制 §3.1 方案 A 保留）。
> 关联：`docs/mady-desktop-standards.md` §14.1 P1-4。

---

## 1. 现状盘点

### 1.1 后端 `pkg/i18n` 结构

`pkg/i18n/` 是完整的轻量翻译框架（零第三方依赖），文件构成：

| 文件 | 职责 |
|------|------|
| `catalog.go` | `Catalog`：线程安全 `map[key]map[Locale]string`；`T(key, args...)` 支持 `%s/%d` 格式化，缺 key 返回 key 原文，缺当前语言回退 **zh-CN**；`LoadYAML`/`LoadDir` 从文件加载翻译；`Global()`/`SetGlobal()`/`T()` 全局快捷方式 |
| `locale.go` | `Locale` 类型 + `LocaleZhCN`/`LocaleEnUS` 常量；`DefaultLocale = zh-CN`；`ParseLocale` 兼容 `zh_CN`/`zh`/`en_US`/`en` 别名 |
| `translations/zh-CN/` `translations/en-US/` | YAML 翻译文件（`common.yaml` 9 键 + `guardrails.yaml` 10 键） |

- **键命名**：点号层级，如 `common.yes`、`guardrail.disclaimer.standard`、`guardrail.level_tag.strict`。
- **加载方式**：`LoadDir("translations/")` 运行时按文件系统路径加载（`doc.go` 用法示意），未见 `go:embed`。
- **当前规模**：共 19 个键（9 common + 10 guardrails），两类 YAML 支持「key.locale」与「分组 + locale 键」两种写法。

**发现（关键）**：全库检索 `i18n.SetGlobal` / `i18n.LoadDir` / `i18n.LoadYAML` 调用点，**均无引导代码**。`guardrails/disclaimer.go` 已通过 `i18n.T(...)` 引用护栏文案，但全局目录未被初始化 → 当前运行时 `T()` 走「缺 key 返回 key 原文」兜底，`guardrail.disclaimer.standard` 这类 key 会原文透出到用户可见文案。**后端 i18n 管线是「有框架、没接电」**，术语源（YAML）与查表机制（Catalog）都存在，缺的是初始化接线。

### 1.2 前端现状

- 技术栈（`desktop/frontend/package.json`）：React 18.3 + TypeScript 5.6 + Vite 5.4 + Tailwind v4 + Zustand + TanStack Query + lucide-react + framer-motion；**依赖表中无任何 i18n 库**。
- 硬编码文案规模（`desktop/frontend/src/`，2026-07-31 ripgrep 实测）：
  - 含中文字符的行：**1658 行**（含代码注释与测试文件——本仓注释约定即中文）；
  - 剔除注释行与测试文件后，落在字符串字面量/JSX 文本中的中文文案行：**约 206 行**（其中 `components/` 179 行，`stores/` 13 行，`theme/` 8 行，`lib/` 5 行，`agui-bridge/` 1 行，`a2ui-renderer/` 0 行）；
  - 数量级判断：面向用户的 UI 文案约 **150-250 条**，分布在 45+ 个文件；文案主体集中在业务组件层，`a2ui-renderer/` 由于文本来自后端 A2UI payload（协议驱动），自身几乎零硬编码文案。

**硬编码文案抽样清单**（非穷尽，取高频组件）：

| 文件 | 典型文案 | 类别 |
|------|---------|------|
| `components/SettingsPanel.tsx` | 「设置」「外观」「AI 服务」「关于」「保存」「亮色 / 深色 / 跟随系统」「已保存，切换将在下一轮新会话中生效」「保存失败，请稍后重试」；**「版本 0.1.0」为硬编码常量** | 面板标题/分组/按钮/Toast/版本展示 |
| `components/Sidebar.tsx` | 「会话」「项目」「文件」「设置」「搜索会话…」「暂无会话」「无匹配会话」「收起侧栏」 | 导航 Tab/空态/placeholder |
| `components/chat/SlashCommandMenu.tsx` | 「清空当前对话」「打开设置面板」「切换主题风格」「专利分析帮助」「新颖性/创造性分析」「审查意见答复起草」「专利无效宣告分析」「专利侵权比对分析」「驳回复审请求书起草」「技术交底书分析」「专利文档撰写」「界面操作」「专利分析」「导出对话为 Markdown」 | 斜杠命令描述（**含专利领域术语**，术语一致性高危区） |
| `components/chat/Composer.tsx` | 「会话导出」等导出文件名与提示 | 工具文案 |
| `components/chat/ChatView.tsx` | 会话视图操作文案（约 69 行含中文） | 视图 |
| `components/ProjectTree.tsx` | 「新建文件夹」「刷新」「项目名称」「文件名称（如 notes.md）」「文件夹（须为空）」「打开现有文件夹作为项目」「新建项目文件夹」 | 文件操作/placeholder/tooltip |
| `components/KnowledgeView.tsx` / `TemplatesView.tsx` / `SkillsView.tsx` | 知识库 / 模板 / 技能视图文案（各 20-44 行含中文） | 视图 |
| `components/StatusBar.tsx` | `v{info?.version ?? '0.1.0'}` —— 版本号来源混杂（后端 `Health().Version` 与前端硬编码兜底并存） | 状态栏 |
| `stores/chat.ts` / `commands.ts` / `settings.ts` | 命令描述、错误消息、推理选项文案 | 逻辑层 |
| `lib/backend.ts` | 服务层错误提示（如 binding 不可用错误） | 服务层 |
| `theme/packs.ts` / `tokens.ts` | 主题包中文名称 | 主题元数据 |

**结论**：前端 i18n 是「全量硬编码」状态，且存在**前后端术语重复维护**——同一专利术语（新颖性/创造性、侵权比对、审查意见答复等）既在后端领域 Agent 提示词/法条库中，又在前端斜杠命令描述中，各写一份，未来必然漂移。

---

## 2. 方案选型

### 2.1 候选方案对比

| 维度 | react-i18next（+i18next） | lingui（js-lingui） | 自研轻量 Context |
|------|--------------------------|---------------------|------------------|
| 成熟度 | 最主流（React 生态事实标准） | 较成熟，Babel/Macro 编译期抽取 | 无生态 |
| 当前版本 | react-i18next 17.0.11 / i18next 26.x（npm registry 实测 2026-07-31）；peer：React ≥16.8（React 18 ✅）、TS ^5 ✅ | lingui 4.x | — |
| 插值/复数 | 开箱即用（`{{var}}`、复数选择器） | ICU MessageFormat，能力最强 | 需自研 |
| 构建链路 | 零构建改动（运行时 JSON 加载） | 需 Babel 插件 / Vite 插件 + `lingui extract` | 零构建改动 |
| 与 `pkg/i18n` 键风格 | 点号/嵌套 JSON 天然兼容 | ICU 键风格，需转换 | 任意 |
| 测试支持 | 组件测试友好（可 mock `useTranslation`） | 同上 | 需自写 |
| 与「克制」哲学匹配度 | 中高（只用核心子集即可） | 中（构建链侵入大） | 高（零依赖） |
| 案例参考 | WailBrew 11 语言、tiny-rdm 12 语言（§13.1） | — | — |

### 2.2 推荐：**react-i18next**

理由（结合项目「克制」哲学与后端术语一致性需求）：

1. **键体系与 `pkg/i18n` 同构**：i18next 的资源键也是点号/嵌套结构，后端 `common.yes`、`guardrail.disclaimer.standard` 可直接映射为前端 `locales/zh-CN.json` 的嵌套键，术语统一成本最低。
2. **插值即用**：桌面端文案大量含动态参数（文件名、版本号、Provider 名），`{{name}}` 插值与后端 `%s` 可在生成脚本中统一约定，不必自研格式化。
3. **生态与案例背书**：目标对标案例（WailBrew 11 语言、tiny-rdm 12 语言）均为成熟 i18n 方案，社区方案 + 版本锁定的组合风险最低（对照 M-DSK-WLS-009 版本锁定原则）。
4. **克制性可控**：只引入 `i18next` + `react-i18next` 两个包与 JSON 资源；语言检测不引入 `i18next-browser-languagedetector` 插件（语言偏好由本地 store 显式管理，避免自动嗅探引入不可预期切换）；复数等高级能力本期用不到就不接。

### 2.3 备选放弃理由

- **lingui**：编译期抽取的工程化收益在「200 条文案、单语言对」量级下不明显，反而增加 Babel/Macro 构建链侵入，与克制哲学冲突。
- **自研 Context**：零依赖诱人，但插值、复数、语言切换重渲染、工具链（key 抽取、缺失检查）全部自建，属于「用开发时间换依赖」，在专业术语一致性要求下不划算。

---

## 3. 术语一致性机制

### 3.1 两个候选机制

**方案 A：单一 JSON/YAML 源 + 生成脚本（推荐）**

- 以 `pkg/i18n/translations/` 为**唯一术语源**（现有 zh-CN/en-US 目录 + 点号键，追加 `patent.term.*` / `legal.term.*` / `desktop.ui.*` 命名空间）。
- 新增生成脚本 `scripts/gen-frontend-i18n.mjs`：
  - 读取 `pkg/i18n/translations/*.yaml` → 扁平点号键 → 输出 `desktop/frontend/src/locales/{zh-CN,en-US}.json`（嵌套 JSON，兼容 i18next `resources` 结构）；
  - 输出 `desktop/frontend/src/locales/keys.json`（key 清单，供测试与 CI 使用）；
  - 对含 `%s` 参数的后端键做约定转换（后端 `%s` → 前端 `{{0}}`/`{{name}}`，或统一约定键内不用参数、改用 `t(key, { n })` 形式——**建议前端一律 `{{name}}` 命名参数，生成脚本只做结构转换不做语义猜译**）。
- CI（`make verify` / `make desktop-test` 挂载）运行 `scripts/check-i18n-keys.sh`：对比 Go 侧加载的键集与前端 JSON 键集，**两端任一缺失/多余键即失败**，从机制上杜绝术语漂移。
- 术语表（专利/法律术语对照）收敛到专门命名空间，前端组件与后端 guardrails/领域文案引用**同一 key**：

```text
patent.term.novelty:           新颖性（en: Novelty）
patent.term.inventiveness:     创造性（en: Inventiveness）
patent.term.claims:            权利要求（en: Claims）
patent.term.oa_response:       审查意见答复（en: OA Response）
patent.term.invalidation:      无效宣告（en: Invalidation）
patent.term.infringement:      侵权比对（en: Infringement Analysis）
patent.term.disclosure:        技术交底书（en: Technical Disclosure）
```

**方案 B：双源人工同步 + 差异检查（兜底）**

- Go 侧与前端各自维护翻译文件，靠 `scripts/check-i18n-keys.sh` 做键集合与值 diff 检查。
- 优点：两端互不阻塞、前端可独立演进；缺点：**人工同步必然漏同步**，术语值不一致只能靠 diff 事后发现，无法从源头预防。

**结论**：推荐方案 A——与后端"单一事实来源"的架构观一致（对照 `docs/mady-desktop-standards.md` 依赖倒置与单一真相源原则）。成本仅一个生成脚本 + 一个 CI 检查，换来前后端专利术语永不漂移。

### 3.2 后端接线缺口（顺带修复项）

第 1.1 节发现 `pkg/i18n` 无引导代码。实施时建议一并补上（一处 `init` 或 `app.go` startup 调用 `i18n.LoadDir` + `i18n.SetGlobal`），否则后端 `i18n.T()` 输出的用户可见文案（护栏免责声明、术语键）仍是 key 原文，与前端译文体系对不上。此修复不属前端 i18n 本体，但**是术语一致性的前提**，列入实施范围 P0。

---

## 4. 实施范围建议

### 4.1 P0 文案清单（面向用户的高频文案）

优先抽取「用户每次使用都会看到 / 影响操作语义」的文案，目标 **40-60 条 key**：

| 分组 | 覆盖组件 | 典型 key（草案） | 条数估算 |
|------|---------|-----------------|---------|
| 导航与结构 | Sidebar、StatusBar | `nav.threads / nav.projects / nav.files / nav.settings` | ~8 |
| 设置面板 | SettingsPanel、ModelSettings、McpServersSettings | `settings.title / settings.appearance / settings.aiService / settings.about / settings.save / theme.light / theme.dark / theme.system` | ~15 |
| 会话操作 | Composer、ChatView、MessageBubble | `composer.placeholder / chat.send / chat.newThread / export.*` | ~10 |
| 斜杠命令（术语区） | SlashCommandMenu | `slash.*`（命令描述，直接引用术语表命名空间） | ~12 |
| 空态与错误 | ProjectTree、TemplatesView、KnowledgeView、backend.ts | `empty.* / error.*`（「暂无会话」「保存失败，请稍后重试」等） | ~10 |
| 通用 | 全部 | `common.confirm / common.cancel / common.loading / common.close` | ~5 |

P0 完成标准：设置面板 + 侧栏 + 斜杠命令菜单三个区域 zh-CN/en-US 完整可切换（对应验收标准 2 的「设置面板原型」）。

### 4.2 切换入口与默认语言

- **默认语言**：`zh-CN`（与 `pkg/i18n.DefaultLocale` 一致，目标用户以中文专业用户为主）。
- **切换入口**：设置面板「外观」区新增「语言 / Language」选择（`zh-CN` / `en-US`），与主题三态并列（参照 W4-T7 暗色三态的位置）。
- **持久化位置**（两级，分层一致）：
  1. 前端 `stores/settings.ts`（`settingsSlice`，zustand `persist` → localStorage，即时生效，参照 W4-T5）；
  2. 后端 `~/.mady/desktop-settings.json` 新增 `locale` 字段（`app_settings.go` 的 `AISettings` 扩展，含语言开关需 `GetSettings`/`SetSettings` 或新增 `SetLocale` binding，走 `backend.ts` 收敛，遵循 M-DSK-WLS-008）——前端启动时以后端为准，保证多端一致。
- **切换行为**：`i18n.changeLanguage(locale)` 即时重渲染（React 18 自动批处理无性能风险），无需重启。

### 4.3 非 P0（明确排除）

- 全量 200 条文案抽取（`a2ui-renderer` 的协议静态文案、`theme/packs.ts` 主题名等）→ 下一迭代；
- `react-i18next` 复数/日期格式化（`Intl` API 场景）、RTL、更多语言（en-US 之外的语种）→ 不进入本期范围。

---

## 5. 工作量估算

评估阶段（W4-T9 排期 0.5-1 人天）：已含本文档 + 抽样统计，**完成**。

实施阶段（若批准，建议 1 个迭代内完成）：

| 任务 | 人天 | 说明 |
|------|------|------|
| 框架接入（`i18next` + `react-i18next` 安装、`src/lib/i18n.ts` 初始化、Provider 挂载、fallback 配置） | 0.5 | 依赖锁版（M-DSK-WLS-009），react-i18next 17.x / i18next 26.x |
| P0 文案抽取与替换（40-60 key，含设置面板原型可切换） | 1-2 | 覆盖第 4.1 清单 |
| 术语一致机制（`scripts/gen-frontend-i18n.mjs` + `check-i18n-keys.sh` + CI 接入） | 0.5-1 | 含 `pkg/i18n` 引导接线修复 |
| 切换入口 + 持久化（`stores/settings.ts` + `AISettings.locale` + binding） | 0.5-1 | 参照 W4-T5 settingsSlice |
| 测试（组件测试 + 语言切换回归 + CI 键一致性） | 0.5 | vitest 组件测试（M-DSK-TST-001/002 就绪后） |
| **合计** | **3-5 人天** | |

---

## 6. 结论

1. **推荐方案**：`react-i18next` + JSON 资源；默认语言 `zh-CN`；术语一致性采用**单一 YAML 源 + 生成脚本 + CI 键检查**（方案 A）；顺带修复 `pkg/i18n` 引导接线缺口。
2. **本期实施**：评估报告（本文档）+ 设置面板单页原型（验收标准 2）**建议本期落地**（含 P0 文案 ~40-60 条，0.5-1 人天评估 + 2-3 人天原型）；全量抽取**放到下一迭代**，与 W4-T5（settingsSlice）、W4-T13（布局/设置持久化）就绪后合并推进，避免两套持久化机制并行。
3. **风险提示**：前端 i18n 与后端 `pkg/i18n` 若分两次实施而不先修后端接线，术语表「单一源」机制将名存实亡——两件事必须同一个迭代闭环。

---

## 附：来源

- 项目内文档：
  - [docs/plans/desktop-next-development-plan.md](../plans/desktop-next-development-plan.md)（W4-T9 小节，P1-4）
  - [docs/mady-desktop-standards.md](../mady-desktop-standards.md)（§13.1 成功案例：tiny-rdm 12 语言 / WailBrew 11 语言；M-DSK-WLS-008/009、M-DSK-TST-001/002）
  - `pkg/i18n/catalog.go` / `locale.go` / `translations/{zh-CN,en-US}/*.yaml`
  - `guardrails/disclaimer.go`（`i18n.T` 引用点）
  - `desktop/frontend/package.json`、`desktop/frontend/src/components/*`（文案抽样）
- 外部事实（npm registry 实测，2026-07-31）：
  - [react-i18next（npm）](https://www.npmjs.com/package/react-i18next) — latest 17.0.11，peer React ≥16.8 / i18next ≥26.2 / TS ^5
  - [i18next（npm）](https://www.npmjs.com/package/i18next) — latest 26.3.6
  - [js-lingui（官网）](https://lingui.dev/) — 编译期抽取方案对比
