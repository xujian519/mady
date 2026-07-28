# 领域扩展层质量审阅报告

> 审阅日期：2026-07-28 | 范围：37 Go 包 | 方法：6 维度 × 14+ 模块逐项检查

---

## 总体结论

| 类别 | 结果 |
|------|------|
| **完整可运行** | ✅ **全部通过** — `go build` / `go vet` / `go test` / `go test -race` / 跨平台编译 / 集成测试 |
| **无断链** | ⚠️ **1 条真断链**（`chat.json` 失效）+ 若干质量改进项 |

---

## 逐维度详细发现

### 维度 A：静态结构完整性

| 检查项 | 结果 |
|--------|------|
| A1. 循环导入 | ✅ `go vet` 0 问题 |
| A2. 未使用代码 (dead code) | ✅ `golangci-lint unused` + `standard` — 0 问题 |
| A3. 接口实现检查 | ⚠️ **37 个接口缺同包编译期检查**（`var _ I = (*T)(nil)`）。非断链，但接口变更时失去编译期保护 |
| A4. 扩展注册完整性 | ✅ **全部 24 个扩展已注册并消费**，0 孤立 |
| A5. go:embed 资产 | ✅ **全部 3 条指令目标存在**（17 模板 + 1 YAML + 4 JSON） |

### 维度 B：编译与类型安全

| 检查项 | 结果 |
|--------|------|
| B1. 跨平台编译 (linux/amd64) | ✅ 通过 |
| B2. 竞态检测 | ✅ 所有领域包 + 集成测试通过，0 data race |
| B3. 多模块一致性 | ✅ go.work 四模块引用正确 |

### 维度 C：测试质量

| 检查项 | 结果 |
|--------|------|
| C1. 覆盖率基线 | ✅ 完整报告见下 |
| C2. 低覆盖模块 | 🔴 5 个包 < 45%（详情见下） |
| C3. 测试数据漂移 | 🔴 2 个高严重性问题（见下） |
| C4. 全局测试 | ✅ 37 包全部 `go test` 通过 |

**覆盖率分布：**

| 范围 | 包数 | 包名 |
|------|------|------|
| **≥ 90%** | 7 | amendment, ipc, novelty, reasoning/wiring, retrieval/domain/sqlite, knowledge/standards, memory/compiler |
| **≥ 80%** | 7 | evidence, guardrails, inventiveness, knowledge/graph, knowledge/loader, psychological, graph |
| **≥ 65%** | 8 | doctmpl, enablement, memory, disclosure, specdrafting, writing, retrieval, reasoning/sqlite |
| **≥ 50%** | 5 | claimdrafting, rules, reasoning, knowledge, knowledge/fileindex |
| **< 45%** | 5 | **🔴 domains (49%), knowledge/risk (43%), knowledge/sqlite (39%), domains/sqlite (23%), infringement (23%), workflows/design (14%)** |

**测试数据漂移严重问题：**
1. **🔴 P0** `domains/claimdrafting/testdata/error-cases.json` — 完全孤儿文件，含 4 组测试用例但无任何测试加载
2. **🔴 P0** `enablement/testdata/enablement_cases.json` — 预期字段从未断言的"半测试"（[已记录](docs/review/2026-07-25-phase2-enablement.md#f-01)）

### 维度 D：依赖链完整性

| 检查项 | 结果 |
|--------|------|
| D1. 分层依赖合规 | ✅ 上层反向依赖 0 违规（无 `tui/` `cmd/` `desktop/` `server/` 导入） |
| D2. 接口依赖原则 | ⚠️ 仅 `guardrails/` 使用 `agentcore/iface`，其余 6 包直接引用 `agentcore` 具体类型 |
| D3. 跨层限制 (.go-arch-lint.yml) | ✅ 0 违规 |

### 维度 E：配置与资源

| 检查项 | 结果 |
|--------|------|
| E1. YAML 语法校验 | ✅ **165/165 文件有效** |
| E2. 模板渲染 | ✅ 所有 doctmpl 模板测试通过 |
| E3. Manifest 一致性 | 🔴 **5 项不一致（见下）** |

**Manifest 一致性问题：**
1. **🔴 P0** `chat.json` 失效 — 缺 `domain` 字段，`name` 格式不合法，被 `LoadManifests` 静默丢弃
2. **🔴 P0** `chat.json` 引用不存在的工具名 `"filesystem"`
3. **🟡 P2** `embedded_manifests.go` 注释与代码不一致（注释称 3 个但嵌入了 4 个）
4. **🟡 P2** `TestLoadManifests_RootManifestsNotDrifted` 持续 skip（比较目标目录不存在）
5. **🟡 P2** 缺乏 manifest → 工具/领域工厂的交叉验证测试

### 维度 F：集成验证

| 检查项 | 结果 |
|--------|------|
| F1. 集成测试 | ✅ `go test -tags=integration -race` 通过 |
| F2. 入口构建 | ✅ `go build ./cmd/mady/` 成功 |
| F3. 入口符号 | ✅ 入口点存在 |

---

## 问题汇总

| 优先级 | 数量 | 类别 | 说明 |
|--------|------|------|------|
| **🔴 P0 — 真断链** | 2 | Manifest / 测试数据 | chat.json 失效被静默丢弃；claimdrafting 测试数据孤儿 |
| **🔴 P0 — 半测试** | 1 | 测试覆盖 | enablement_cases.json 加载但不断言预期结果 |
| **🟡 P1 — 质量** | 1 | 依赖 | 6/7 领域包未使用 `agentcore/iface` 接口层 |
| **🟡 P1 — 覆盖** | 5 | 测试覆盖 | 5 个包覆盖率 < 45% |
| **🟡 P2 — 维护** | 3 | Manifest | 死 JSON、skip 测试、缺交叉验证 |
| **🟢 P3 — 风格** | 37 | 接口 | 缺编译期接口实现检查（非断链，推荐） |

---

## 改进建议

**P0 优先修复：**
1. 删除或修复 `agentcore/manifests/chat.json`（若不再需要则直接删除，同时更新注释）
2. 为 `domains/claimdrafting/testdata/error-cases.json` 编写测试用例
3. 连通 `enablement` 测试断言链

**P1 系统改进：**
4. 将 `domains/sqlite`、`knowledge/sqlite` 等低覆盖包的基础功能补测至 ≥ 60%
5. 逐步迁移领域包到 `agentcore/iface` 接口依赖

**P2 持续维护：**
6. 恢复 `manifest` 漂移检测 testdata 目录
7. 添加 manifest → 工具/工厂交叉验证
8. 删除根目录漂移的 `testdata/` 文件（`test.txt`, `test.csv`, `test.docx`）

---

*审阅范围：`domains/` · `guardrails/` · `knowledge/` · `retrieval/` · `memory/` · `disclosure/` · `psychological/` · `graph/` · `workflows/` · `styles/` · `doc-templates/` · `agentcore/manifests/`*
