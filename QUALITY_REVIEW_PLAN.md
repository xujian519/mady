# 领域扩展层质量审阅计划

> 基线：37 个 Go 包在 `domains/`、`guardrails/`、`knowledge/`、`retrieval/`、`memory/`、`disclosure/`、`psychological/`、`graph/`、`workflows/` 下均通过 `go build ./...`、`go vet ./...`、`go test ./...`（0 失败）。

## 审阅范围（6 大维度 × ~14+ 模块）

### 维度 A：静态结构完整性（断链检查）

| 检查项 | 方法 |
|--------|------|
| A1. **循环导入** | `go vet` → 已覆盖，0 问题。补充 `go mod verify` |
| A2. **未使用符号（dead code）** | `golangci-lint run --disable-all -E unused` 扫描各模块 |
| A3. **接口实现完备性** | 对每个导出的 interface 做 `grep` 确认所有实现者的编译期检查（`var _ Foo = (*Bar)(nil)`） |
| A4. **扩展注册完整性** | 确认 `tools/tools.go`、`domains/*.go` 中所有 Extension/LifecycleHook 均已注册，未被注释/遗漏 |
| A5. **嵌入资源可访问性** | 确认 `go:embed` 路径指向的实际文件存在，未被移动/删除 |

### 维度 B：编译与类型安全

| 检查项 | 方法 |
|--------|------|
| B1. **全量交叉编译** | `GOOS=linux GOARCH=amd64 go build ./...` 验证跨平台无问题 |
| B2. **竞态检测** | `go test -race ./domains/... ./guardrails/...` 等，确认并发安全 |
| B3. **go.work 多模块一致性** | 确认根模块 + tools + tui + desktop 四者间 import 路径正确 |

### 维度 C：测试质量

| 检查项 | 方法 |
|--------|------|
| C1. **覆盖率基线** | `go test -coverprofile=coverage.out ./...` 收集当前覆盖率 |
| C2. **缺失测试的模块** | 标记测试文件数 < 源码文件数 30% 的包为高风险 |
| C3. **测试数据漂移** | 确认 `testdata/` 中文件未被源码引用的已废弃路径引用 |
| C4. **模糊测试 / 表格驱动测试** | 扫描复杂逻辑函数是否缺少边界用例 |

### 维度 D：依赖链完整性

| 检查项 | 方法 |
|--------|------|
| D1. **分层依赖合规** | 验证领域层不反向导入 agentcore（`grep 'mady/agentcore' domains/*.go`），早期违规标记 |
| D2. **内部包可见性** | 确认 `internal/` 包的消费者都在 allowed 模块内 |
| D3. **模拟/桩完整性** | 测试中使用的 mock/stub 是否随接口变更而更新 |

### 维度 E：配置与资源

| 检查项 | 方法 |
|--------|------|
| E1. **YAML 配置有效性** | `domains/rules/data/` + `styles/` 中所有 `.yaml` 语法校验 |
| E2. **模板渲染完整性** | `doc-templates/` + `domains/doctmpl/templates/` 模板语法校验 |
| E3. **领域 Manifest** | `agentcore/manifests/` JSON 与 `domains/` 中实际配置一致性 |

### 维度 F：运行时集成完整性

| 检查项 | 方法 |
|--------|------|
| F1. **集成测试** | `go test ./integration/...` 验证端到端无断链 |
| F2. **入口子命令** | `cmd/mady/` 各子命令（tui/serve/acp/eval/evidence/util）均能正常启动 |
| F3. **入口构建** | `go build -o /dev/null ./cmd/mady/` 成功 |
| F4. **Smoke test**（可选） | 二进制启动 + 立即退出验证 |

## 执行顺序

```
Phase 1: 基线确认（已完成）
  └── go build / go vet / go test → 全部通过 ✓

Phase 2: 静态分析（维度 A + B）
  ├── 2a. unused code lint
  ├── 2b. 接口实现 + 注册点审计
  ├── 2c. go:embed 资产核对
  ├── 2d. 跨平台编译 + race test
  └── 2e. 分层依赖合规审计

Phase 3: 测试与覆盖（维度 C）
  ├── 3a. 覆盖率报告生成
  ├── 3b. 低覆盖模块标记与补全
  └── 3c. 测试数据漂移检查

Phase 4: 配置与资源（维度 E）
  ├── 4a. YAML 语法全面校验
  ├── 4b. 模板渲染 dry-run
  └── 4c. manifest 一致性核对

Phase 5: 集成验证（维度 F）
  ├── 5a. 集成测试套件运行
  ├── 5b. 入口构建与 smoke test
  └── 5c. 全量竞态检测

Phase 6: 报告生成
  └── 汇总发现 → 按 P0/P1/P2 分级 → 修复建议
```

## 交付物

1. **本计划** → 评审和执行指引
2. **逐模块检查清单** → 每个模块独立条目，标注通过/失败/异常
3. **问题清单** → 所有发现项按优先级排序
4. **覆盖率报告** → 当前基线 + 改进建议
5. **最终审阅报告** → 是否通过、遗留风险、后续建议

---

> 准备就绪后，我将按 Phase 2→6 顺序逐项执行，每完成一个维度输出一份状态快照。
