# 质量门禁审计清单

> 审计日期：2026-07-20
> 审计范围：本地仓库门禁、CI 门禁、GitHub 远程保护、规范与文档一致性
> 审计目标：判断当前质量门禁是否符合“离二边，行中道”的工程目标，是否足以支撑长期发展

---

## 使用说明

- 本清单按四个维度展开：`本地门禁`、`CI 门禁`、`GitHub 后台保护`、`规范一致性`
- 每项包含三部分：`当前状态`、`核验动作`、`结论/建议`
- 状态说明：
  - `已满足`：仓库内已有充分证据支持
  - `部分满足`：已有机制，但存在缺口或口径不一致
  - `待核验`：需要到 GitHub 仓库后台或实际运行结果中确认
  - `建议补齐`：当前不是阻断问题，但建议纳入近期治理
- 优先级说明：
  - `P0`：必须尽快确认或修复，否则门禁闭环不完整
  - `P1`：应在近期补齐，否则长期会持续产生摩擦或漂移
  - `P2`：持续优化项

---

## 审计结论摘要

| 维度 | 当前判断 | 结论 |
|------|----------|------|
| 本地门禁 | 部分满足 | 基础完整，但默认入口低于真实提交标准 |
| CI 门禁 | 已有较强覆盖 | 主体扎实，AI 加严门与多模块覆盖仍有缺口 |
| GitHub 后台保护 | 待核验 | 仓库内看不到 required checks / required reviews 是否真实开启 |
| 规范一致性 | 部分满足 | 规则整体成熟，但存在文档重复和口径分叉 |

**总评**：

- 当前仓库已经具备较成熟的质量门禁骨架，方向正确。
- 主要问题不是“没有门禁”，而是“默认入口、执行口径、远程闭环”尚未完全统一。
- 若 GitHub 后台已正确启用分支保护，则整体可评为“合理且适合长期发展”。
- 若 GitHub 后台未启用 required checks / required reviews，则当前仍属于“有检查流程，但未完全形成强门禁”。

---

## 一、本地门禁审计

### 1.1 统一入口与真实标准

- [x] `P0` 存在统一的本地构建入口
  - 当前状态：`已满足`
  - 核验动作：确认 [Makefile](file:///Users/xujian/projects/Mady/Makefile#L21-L25) 存在一键命令
  - 结论/建议：当前有 `make all = vet + build + test`

- [ ] `P0` 默认一键命令等于“提交前真实标准”
  - 当前状态：`部分满足`
  - 核验动作：对比 [Makefile](file:///Users/xujian/projects/Mady/Makefile#L21-L25) 与 [AGENTS.md](file:///Users/xujian/projects/Mady/AGENTS.md#L18-L23)、[GO-DEVELOPMENT-STANDARDS.md](file:///Users/xujian/projects/Mady/docs/GO-DEVELOPMENT-STANDARDS.md#L905-L913)
  - 结论/建议：`make all` 不含 `test-race` 和 `lint`，建议新增 `make verify`

### 1.2 多模块覆盖

- [x] `P0` 本地命令已覆盖 root 与 `tools/`
  - 当前状态：`已满足`
  - 核验动作：检查 [Makefile](file:///Users/xujian/projects/Mady/Makefile#L31-L33)、[Makefile](file:///Users/xujian/projects/Mady/Makefile#L54-L60)、[Makefile](file:///Users/xujian/projects/Mady/Makefile#L114-L125)
  - 结论/建议：多模块工作区问题已被显式处理，这是当前门禁设计的优点

### 1.3 pre-commit / commit-msg 钩子

- [x] `P0` pre-commit 已覆盖格式化与基础静态检查
  - 当前状态：`已满足`
  - 核验动作：检查 [.pre-commit-config.yaml](file:///Users/xujian/projects/Mady/.pre-commit-config.yaml#L13-L42)
  - 结论/建议：已覆盖 `gofmt`、`goimports`、`go vet`、`golangci-lint`

- [ ] `P1` lint 缺失时本地应阻断而非仅提示
  - 当前状态：`部分满足`
  - 核验动作：检查 [Makefile](file:///Users/xujian/projects/Mady/Makefile#L118-L125)
  - 结论/建议：当前 `golangci-lint` 未安装时仅提示不失败，适合降低新手摩擦，但会削弱一致性

- [x] `P0` commit-msg 已覆盖提交规范
  - 当前状态：`已满足`
  - 核验动作：检查 [.pre-commit-config.yaml](file:///Users/xujian/projects/Mady/.pre-commit-config.yaml#L43-L54) 与 [commitlint.config.js](file:///Users/xujian/projects/Mady/commitlint.config.js#L1-L10)
  - 结论/建议：Conventional Commits 已受控，且中文标题长度做了本地化放宽

- [x] `P0` 敏感路径本地阻断已存在
  - 当前状态：`已满足`
  - 核验动作：检查 [check-sensitive-paths.sh](file:///Users/xujian/projects/Mady/scripts/check-sensitive-paths.sh#L16-L46) 和 [check-sensitive-paths.sh](file:///Users/xujian/projects/Mady/scripts/check-sensitive-paths.sh#L161-L174)
  - 结论/建议：`AI + 敏感路径` 会直接失败，这一条是关键硬门禁

- [ ] `P1` 首次克隆后漏装钩子的风险已被机制消除
  - 当前状态：`部分满足`
  - 核验动作：检查 [AGENTS.md](file:///Users/xujian/projects/Mady/AGENTS.md#L35-L47) 与 [CONTRIBUTING.md](file:///Users/xujian/projects/Mady/CONTRIBUTING.md#L255-L286)
  - 结论/建议：文档提醒充分，但仍依赖开发者手动安装，建议在 onboarding 文档中进一步突出 `make install-hooks`

---

## 二、CI 门禁审计

### 2.1 主 CI 覆盖范围

- [x] `P0` PR 与 `main` push 都会触发主 CI
  - 当前状态：`已满足`
  - 核验动作：检查 [ci.yml](file:///Users/xujian/projects/Mady/.github/workflows/ci.yml#L3-L7)
  - 结论/建议：主线与合并前链路都有自动校验

- [x] `P0` CI 已覆盖静态检查
  - 当前状态：`已满足`
  - 核验动作：检查 [ci.yml](file:///Users/xujian/projects/Mady/.github/workflows/ci.yml#L28-L49)
  - 结论/建议：root 与 `tools/` 都跑 `golangci-lint`

- [x] `P0` CI 已覆盖依赖整洁性
  - 当前状态：`已满足`
  - 核验动作：检查 [ci.yml](file:///Users/xujian/projects/Mady/.github/workflows/ci.yml#L68-L93)
  - 结论/建议：root 与 `tools/` 都跑 `go mod tidy -diff`

- [x] `P0` CI 已覆盖 build / test / race
  - 当前状态：`已满足`
  - 核验动作：检查 [ci.yml](file:///Users/xujian/projects/Mady/.github/workflows/ci.yml#L95-L130)
  - 结论/建议：root/tools × ubuntu/macos 都有覆盖，属于比较稳健的配置

- [x] `P1` CI 已覆盖集成测试
  - 当前状态：`已满足`
  - 核验动作：检查 [ci.yml](file:///Users/xujian/projects/Mady/.github/workflows/ci.yml#L142-L153)
  - 结论/建议：当前仅 Ubuntu 跑 integration，可接受

### 2.2 专项门禁与 AI 加严

- [x] `P1` 已存在专项质量门
  - 当前状态：`已满足`
  - 核验动作：检查 [Makefile](file:///Users/xujian/projects/Mady/Makefile#L73-L91) 与 [ci.yml](file:///Users/xujian/projects/Mady/.github/workflows/ci.yml#L50-L67)
  - 结论/建议：`disclosure` dry-run gate 与 eval benchmark 已构成领域专项门禁

- [ ] `P1` AI 参与 PR 的加严门禁足够稳健
  - 当前状态：`部分满足`
  - 核验动作：检查 [ai-code-quality.yml](file:///Users/xujian/projects/Mady/.github/workflows/ai-code-quality.yml#L16-L72)
  - 结论/建议：现有策略合理，但 AI 识别依赖 `Co-authored-by` 文本，存在漏检面

- [ ] `P0` AI 加严门与多模块现实保持一致
  - 当前状态：`部分满足`
  - 核验动作：检查 [ai-code-quality.yml](file:///Users/xujian/projects/Mady/.github/workflows/ai-code-quality.yml#L54-L72)
  - 结论/建议：当前只显式覆盖根模块，建议补上 `tools/` 子模块 lint/test

### 2.3 覆盖率与发布门

- [ ] `P1` 覆盖率视图反映全仓真实质量
  - 当前状态：`部分满足`
  - 核验动作：检查 [ci.yml](file:///Users/xujian/projects/Mady/.github/workflows/ci.yml#L132-L140) 与 [codecov.yml](file:///Users/xujian/projects/Mady/codecov.yml#L1-L16)
  - 结论/建议：当前仅上传 root/ubuntu 覆盖率，不含 `tools/` 与 integration

- [ ] `P1` 发布流程具备独立前置校验
  - 当前状态：`部分满足`
  - 核验动作：检查 [release.yml](file:///Users/xujian/projects/Mady/.github/workflows/release.yml#L1-L29)
  - 结论/建议：当前 tag 发布直接走 goreleaser，本身没错，但前提是上游保护足够强

- [x] `P2` 已存在依赖和长期回归巡检
  - 当前状态：`已满足`
  - 核验动作：检查 [dependabot.yml](file:///Users/xujian/projects/Mady/.github/dependabot.yml#L1-L27) 与 [scheduled-eval.yml](file:///Users/xujian/projects/Mady/.github/workflows/scheduled-eval.yml#L1-L47)
  - 结论/建议：weekly dependabot 与 scheduled eval 对长期质量有正面价值

---

## 三、GitHub 后台保护审计

> 这一部分无法仅凭仓库文件确认，必须进入 GitHub 仓库后台核验。

- [ ] `P0` `main` 已禁止直接 push
  - 当前状态：`待核验`
  - 核验动作：在 GitHub `Settings -> Branch protection / Rulesets` 中确认
  - 结论/建议：若允许直接 push，则 `commitlint` 和部分 PR 语义门禁会被绕过

- [ ] `P0` 已启用 `Require a pull request before merging`
  - 当前状态：`待核验`
  - 核验动作：检查分支保护规则
  - 结论/建议：这是远程门禁闭环的关键前提

- [ ] `P0` 已启用 `Require approvals`
  - 当前状态：`待核验`
  - 核验动作：确认至少 1 位非提交者 review
  - 结论/建议：若无此项，则文档里的 L2/L3/L4 审查分级只能算流程建议

- [ ] `P0` 已将关键 CI job 设为 required checks
  - 当前状态：`待核验`
  - 核验动作：确认至少包含 `lint`、`mod-tidy`、`test (...)`、`integration`、`AI Code Quality Gate`
  - 结论/建议：这是“CI 会跑”和“CI 能拦”的分水岭

- [ ] `P1` 已限制管理员绕过保护
  - 当前状态：`待核验`
  - 核验动作：检查 branch protection advanced settings
  - 结论/建议：如允许管理员绕过，需至少形成例外审批约定

- [ ] `P1` 已要求分支与 base 保持最新后才能合并
  - 当前状态：`待核验`
  - 核验动作：检查是否启用“Require branches to be up to date”
  - 结论/建议：可减少“旧绿灯”带来的合并后回归

- [ ] `P1` 已禁用 force push 与删除受保护分支
  - 当前状态：`待核验`
  - 核验动作：检查保护规则
  - 结论/建议：建议作为标准保护项启用

- [ ] `P1` 已配置 `CODEOWNERS`
  - 当前状态：`建议补齐`
  - 核验动作：仓库内搜索 `CODEOWNERS`
  - 结论/建议：当前未发现 `CODEOWNERS`，建议为 `tools/`、`domains/`、`agentcore/permission/`、`guardrails/guardian/`、敏感路径建立 owner

---

## 四、规范一致性审计

### 4.1 文档与实际执行的一致性

- [ ] `P0` 文档中的提交标准与真实门禁一致
  - 当前状态：`部分满足`
  - 核验动作：对比 [AGENTS.md](file:///Users/xujian/projects/Mady/AGENTS.md#L18-L23) 与 [PULL_REQUEST_TEMPLATE.md](file:///Users/xujian/projects/Mady/.github/PULL_REQUEST_TEMPLATE.md#L33-L47)
  - 结论/建议：当前 PR 模板仍主要写 `go vet ./...`、`go test ./...`，未显式体现多模块与 `-race`

- [ ] `P0` 多模块现实在贡献文档中充分体现
  - 当前状态：`部分满足`
  - 核验动作：检查 [CONTRIBUTING.md](file:///Users/xujian/projects/Mady/CONTRIBUTING.md#L26-L48)
  - 结论/建议：贡献文档示例仍偏单模块，应统一为 `Makefile` 封装或显式 root + tools

### 4.2 单一真相源

- [ ] `P1` 敏感路径清单只有一个权威源
  - 当前状态：`部分满足`
  - 核验动作：对比 [AGENTS.md](file:///Users/xujian/projects/Mady/AGENTS.md#L89-L115)、[SECURITY.md](file:///Users/xujian/projects/Mady/SECURITY.md#L62-L89)、[GO-DEVELOPMENT-STANDARDS.md](file:///Users/xujian/projects/Mady/docs/GO-DEVELOPMENT-STANDARDS.md#L922-L947)、[check-sensitive-paths.sh](file:///Users/xujian/projects/Mady/scripts/check-sensitive-paths.sh#L16-L46)
  - 结论/建议：脚本已是权威源，但文档仍存在多处重复抄录，长期有漂移风险

- [ ] `P1` 变更记录职责边界足够清晰
  - 当前状态：`部分满足`
  - 核验动作：核对 `CHANGELOG.md` 与 [AI_CHANGELOG.md](file:///Users/xujian/projects/Mady/docs/decisions/AI_CHANGELOG.md)
  - 结论/建议：建议明确一个记录“用户可见变化”，一个记录“AI 决策、原因、风险”

### 4.3 架构与审查分级

- [x] `P0` 架构边界有明确文档支持
  - 当前状态：`已满足`
  - 核验动作：检查 [AGENTS.md](file:///Users/xujian/projects/Mady/AGENTS.md#L63-L69) 与 [chat-assistant-architecture.md](file:///Users/xujian/projects/Mady/docs/chat-assistant-architecture.md#L13-L50)
  - 结论/建议：这一块是规范体系的强项，应继续保持

- [x] `P0` 审查分级有清晰定义
  - 当前状态：`已满足`
  - 核验动作：检查 [CONTRIBUTING.md](file:///Users/xujian/projects/Mady/CONTRIBUTING.md#L236-L253)
  - 结论/建议：L1-L4 分级合理，建议与 GitHub 后台保护对齐

- [ ] `P2` 新贡献者可通过一页文档快速上手全部门禁
  - 当前状态：`建议补齐`
  - 核验动作：检查当前是否已有开发者速查表
  - 结论/建议：建议后续补一页式 `开发者门禁速查表`

---

## 五、优先整改清单

### P0：先补齐闭环

- [ ] 在 GitHub 后台确认并开启 `禁止直接推 main`
- [ ] 在 GitHub 后台确认并开启 `Require pull request before merging`
- [ ] 在 GitHub 后台确认并开启 `Require approvals`
- [ ] 在 GitHub 后台将关键 CI job 设为 `required status checks`
- [x] 新增 `make verify`，使其真正代表提交前标准 ✅（已存在并推广至 AGENTS.md / CLAUDE.md / PR 模板）
- [x] 让 `AI Code Quality Gate` 显式覆盖 `tools/` 子模块 ✅（已覆盖 lint + test）

### P1：统一口径，降低长期漂移

- [x] 更新 [CONTRIBUTING.md](file:///Users/xujian/projects/Mady/CONTRIBUTING.md) 的构建与测试示例，改成多模块口径 ✅（已修复 `go build ./....` 笔误，tools 计数 35→60）
- [x] 更新 [PULL_REQUEST_TEMPLATE.md](file:///Users/xujian/projects/Mady/.github/PULL_REQUEST_TEMPLATE.md) 的检查项，显式加入 `-race` 与 `tools/` ✅
- [x] 引入 `CODEOWNERS`，为敏感路径和核心模块建立默认评审归属 ✅（已存在 `.github/CODEOWNERS`）
- [x] 收敛敏感路径表格，保留脚本为唯一权威，文档改为引用式摘要 ✅（所有文档已声明脚本为权威源）
- [ ] 明确 `CHANGELOG.md` 与 `AI_CHANGELOG.md` 的职责边界

### P2：持续优化

- [ ] 评估是否把 `coverage` 扩展为 root + `tools/` 的联合视图
- [ ] 评估是否为 release 增加轻量发布前校验
- [x] 补充一页式开发者门禁速查表 ✅（已创建 `docs/developer-quickstart.md`）

---

## 六、建议结论模板

可直接用于 issue、周报或阶段总结：

> 当前仓库已具备较完整的本地与 CI 质量门禁，整体方向正确，能够支撑项目持续演进。
> 本地门禁口径已统一（`make verify` 已成为各文档推荐入口），AI Code Quality Gate 已覆盖 tools/ 子模块，
> 敏感路径权威源已收敛，CODEOWNERS 已配置。主要剩余缺口：GitHub 远程分支保护需后台确认。

---

## 七、审计后的总体判断

- 这套门禁体系`总体合理`
- 它已经接近“恰到好处的抽象，克制的工程实践”
- 目前最重要的不是继续加码，而是把现有规则变成`更一致、更可执行、更少漂移的一套系统`
