# 架构债修复计划 — 基于 Brooks-Lint 架构审计（2026-08-22）

> 审计来源：Brooks-Lint Architecture Audit，Health Score 50/100
> （2 Critical / 4 Warning）。本计划按杠杆率与风险排序，每个任务遵守
> AGENTS.md「单次改动 3-5 个文件」约束；机械性 import 重写豁免并分批提交。

## 0. 侦查结论（计划依据）

| 发现 | 关键事实 | 修复难度 |
|------|---------|---------|
| C1 knowledge↔retrieval 循环 | `retrieval/domain/sqlite/patent_retriever.go` 仅用 `SQLiteStore` 的 2 个方法：`FTSSearch(term, topK)`、`GetChunksByDocID(docID, n)`。该文件头注释自述"never on knowledge/sqlite"，自己违反自己的边界 | 低（1 文件） |
| C2 domains 根包直连 sqlite 驱动 | case_index 族 8 文件 ~1021 行；6 处重复 `_ "modernc.org/sqlite"` 空导入；根包内仅 `case_extension.go` 引用 CaseIndex；外部调用方为 bootstrap/setup_steps.go、setup.go、cmd/mady/tui_session_commands.go | 中（分三步） |
| W1 pkg→agentcore | `pkg/agentconfig/provider.go` 返回 `agentcore.Provider`，本质是组装包；被 cmd(5)/bootstrap(6)/desktop(4)/acp(1)/example(1) 引用 | 低-中（机械迁移） |
| W2 tools/evaluate 反向依赖 domain | `tools/patent_eval.go` 经 Config 持有 `*rules.Engine` 具体类型；`evaluate/reasoning_pattern_coverage.go:49` 调用 `patent.AllPatterns()`（592 行数据函数） | 中 |
| W3 domains 上帝包 | 根包 45 文件 ≥8 类职责；巨型函数 AllPatterns() 592 行、buildSlashRegistry() 495 行 | 高（最后做） |
| W4 go.work 双向引用 | root require tools 且 tools import root 的 agentcore/pkg/retrieval/domains。切断 domains 边后双向仍在（agentcore/pkg/retrieval 边） | 需架构决策 |

**勘误**：审计图中 `graph ↔ agentcore` 循环经复核仅存在于测试文件
（graph_test.go、pregel_test.go），生产代码无循环，不列入修复范围。

---

## Phase 1 — 解除 knowledge↔retrieval 循环（C1）🔴

### 任务 T1.1：patent_retriever 接口化
- **改动文件**：
  - `retrieval/domain/sqlite/patent_retriever.go`
    1. 在包内定义最小接口（Seam 注入点）：
       ```go
       // FTSStore is the slice of the knowledge store this retriever needs.
       type FTSStore interface {
           FTSSearch(term string, topK int) ([]retrieval.ScoredChunk, error)
           GetChunksByDocID(docID string, limit int) ([]retrieval.ScoredChunk, error)
       }
       ```
       （方法签名以 `knowledge/sqlite.SQLiteStore` 实际签名为准）
    2. `store *sqlite.SQLiteStore` → `store FTSStore`
    3. 删除 `"github.com/xujian519/mady/knowledge/sqlite"` import
    4. 更新包头注释：composition seam 现在通过接口绑定
- **预期影响**：调用方 `bootstrap/init_reasoning.go`、`cmd/mady/server.go`
  传入的 `*sqlite.SQLiteStore` 隐式满足接口，理论零改动；若编译报错按报错微调。
- **验证**：
  - [ ] `go build ./...`
  - [ ] `grep -rn "mady/knowledge" retrieval/ --include='*.go' | grep -v _test` 输出为空
  - [ ] `go test -race ./retrieval/... ./knowledge/... ./bootstrap/...`

### 任务 T1.2：回归确认 + 记录
- **验证**：
  - [ ] `make all` 通过
  - [ ] `mady eval --suite=retrieval`（如存在该套件）分数不低于基线 docs/evaluation-baseline-v0.8.md
  - [ ] 按 AGENTS.md 追加 changelog 条目

---

## Phase 2 — case_index 去驱动化 + 抽离子包（C2，兼做 W3 第一刀）🔴

> 注意：`domains/sqlite/checkpoint_store.go`、`domains/reasoning/sqlite/` 属
> Checkpoint 相关敏感路径，本阶段**不动**，另行立项。

### 任务 T2.1：消除重复驱动注册（过渡性小改）
- **改动文件**：`domains/case_index_doc.go`、`case_index_event.go`、
  `case_index_lifecycle.go`、`case_index_path.go`、`case_index_search.go`、
  `case_index_crud.go` — 各删一行 `_ "modernc.org/sqlite"`，
  仅保留 `case_index.go` 一处。
- **验证**：
  - [ ] `grep -c 'modernc.org/sqlite' domains/case_index*.go` 总计 = 1
  - [ ] `go build ./...`

### 任务 T2.2：case 族整体迁至 `domains/caseindex/` 子包
- **前置侦查**（执行时先做）：`codegraph_impact CaseIndex` 确认全部引用面。
- **改动文件**（机械迁移 + import 重写）：
  1. `git mv domains/case_index*.go domains/caseindex/`（package 改名 `caseindex`）
  2. `domains/case_extension.go` 改 import 新包
  3. `bootstrap/setup.go`、`bootstrap/setup_steps.go`、`cmd/mady/tui_session_commands.go` 改 import
- **效果**：domains 根包不再含 SQL；caseindex 成为显式持久化 adapter 子包
  （与 domains/sqlite 同级定位），根包职责 −1021 行。
- **验证**：
  - [ ] `grep -l 'modernc.org/sqlite' domains/*.go` 输出为空（根目录平铺文件）
  - [ ] `make verify` 全过（lint+build+test-race 四模块）
  - [ ] 手工冒烟：任意目录启动 `mady tui`，案件上下文探测仍工作（CWD 即工作区）

### 任务 T2.3：（可选加固）CaseIndex 提取 `iface.CaseIndexStore` 接口
- 若 bootstrap/cmd 只用 FindByPath/CRUD 等少量方法，在 `domains/iface/`
  定义接口、bootstrap 持有接口，使未来更换存储引擎不动领域代码。
- **触发条件**：T2.2 后若 `bootstrap` 对 `*caseindex.CaseIndex` 的方法调用 ≤8 个则执行，否则 defer。

---

## Phase 3 — pkg/agentconfig 迁回组装层（W1）🟡

### 任务 T3.1：`pkg/agentconfig` → `bootstrap/agentconfig`
- **理由**：BuildProvider 返回 `agentcore.Provider`，是纯组装逻辑；
  bootstrap 是组合根，允许依赖内核。pkg 回归纯工具库定位。
- **批次拆分**（机械 rename，分两批提交控制单次跨度）：
  - 批次 A（根模块内）：`git mv` + 更新 cmd(5)/bootstrap 内部引用
  - 批次 B（跨模块）：desktop(4)、acp/server_app.go(1)、example(1) 改 import
- **验证**：
  - [ ] `grep -rn "pkg/agentconfig" --include='*.go' .` 输出为空
  - [ ] 根模块 `go build ./...` + `(cd desktop && go build ./...)` + `(cd tui && go build ./...)`
  - [ ] desktop 冒烟：应用可启动、模型设置面板正常读取 provider 配置

---

## Phase 4 — infra→domain 反向边接口化（W2）🟡

### 任务 T4.1：tools 切断对 domains/rules 的依赖
- **改动文件**：`tools/patent_eval.go` + Engine 组装点（bootstrap 或 cmd）
- **方案**：Config 中 `*rules.Engine` 改为 tools 本地定义的最小接口
  （实际只用到 `Evaluate(content, "", required)` 一类方法，执行时以真实方法集为准），
  `rules.Engine` 隐式满足；组装点注入具体引擎。
- **验证**：
  - [ ] `grep -rn "mady/domains" tools/ --include='*.go' | grep -v _test` 输出为空
  - [ ] `go test -race ./tools/...`（tools 为独立模块，须 cd tools 单独跑）

### 任务 T4.2：evaluate 切断对 domains/workflows/patent 的依赖
- **改动文件**：`evaluate/reasoning_pattern_coverage.go` + 调用方
- **方案**：覆盖率统计所需输入（pattern 名称/元数据列表）改为参数注入或
  `PatternSource` 函数类型；由 CLI 入口（cmd/mady eval 或 bootstrap）注入
  `patent.AllPatterns`。evaluate 保持领域无关。
- **验证**：
  - [ ] `grep -rn "mady/domains" evaluate/ --include='*.go' | grep -v _test` 输出为空
  - [ ] `mady eval --format=json` 输出与改造前 diff 为空（行为不变）

### 任务 T4.3：go.work 结构决策（W4）⚠️ NEEDS CLARIFICATION
切断 domains 边后，tools 仍 import 根模块 agentcore/pkg/retrieval，
root↔tools 双向 require 依旧存在。彻底解法二选一，**需人工决策**：
- 方案 A：把 `tools/` 上收回根模块（放弃独立 module，最简单）
- 方案 B：把 tools 依赖的根模块部分（path 沙箱等）下沉为独立叶子 module，
  使依赖单向 root→leafstools
- **暂停点**：此项涉及仓库布局与发布流程，未经人工确认不执行。

---

## Phase 5 — domains 上帝包继续瘦身（W3）🟢

> 每个子任务独立成 PR，顺序不限；T2.2 已完成 case 族第一刀。

| 任务 | 内容 | 验证 |
|------|------|------|
| T5.1 | `deadline_calculator.go` + `deadline_extension.go` → `domains/deadline/` | grep 根包无 deadline 符号；make all |
| T5.2 | `audit_alias.go`、`audit_extension.go` 并入既有 `domains/audit/` 子包 | 同上 |
| T5.3 | `AllPatterns()` 数据体拆至 `reasoning_patterns_data.go`（纯移动不改内容） | `mady eval` 输出不变 |
| T5.4 | `cmd/mady/slash_commands.go buildSlashRegistry()`（495 行）按命令组拆为 registerXxx 函数表 | `go test ./cmd/...` + TUI 手工冒烟 `/help` |

---

## 总检查清单（每 Phase 收尾必跑）

```bash
make verify                 # lint + build + test-race，覆盖四个模块
(cd tools && go test ./...) # tools 独立模块双保险
(cd tui   && go build ./...)
(cd desktop && go build ./...)
```

循环依赖断言（Phase 1/4 后应为空）：

```bash
grep -rn "mady/knowledge"      retrieval/ --include='*.go' | grep -v _test
grep -rn "mady/domains"        tools/ evaluate/ --include='*.go' | grep -v _test
grep -rn "mady/agentcore"      pkg/ --include='*.go' | grep -v _test
grep -l  "modernc.org/sqlite"  domains/*.go
```

变更记录（AGENTS.md 强制）：

```bash
go run scripts/changelog/main.go --type=refactor --scope=<模块> \
  --title="..." --body="..."
```

安全红线自查：
- [ ] 未触碰敏感路径表中的文件（handoff/levels/router/approval/tools/path.go 等）
- [ ] Checkpoint 存储（domains/sqlite/checkpoint_store.go 等）零改动
- [ ] 无新增 `./` 相对路径默认值（资源定位走 util.MadyHome()）

## 建议执行顺序与节奏

```
T1.1→T1.2（半天）→ T2.1→T2.2（1 天）→ T3.1（半天）
→ T4.1→T4.2（1 天）→ [T4.3 人工决策暂停] → Phase 5 择机穿插
```

每 Phase 结束打一次 git tag（如 `arch-fix-phase1`），保证可独立回滚。
