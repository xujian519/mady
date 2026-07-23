# 说明书撰写模块 & 权利要求书撰写模块 — 全量质量审阅报告

> 审阅日期：2026-07-23
> 审阅范围：`domains/specdrafting/`（15 文件，2,980 行）+ `domains/claimdrafting/`（17 文件，3,974 行）
> 基线 Commit：当前 `main` 分支 HEAD
> 审阅方法：6 阶段基线验证 + 4 路并行深度审阅 + 交叉验证

---

## 总体评估

### 健康度评分

| 维度 | 评分 | 说明 |
|------|------|------|
| 架构设计 | ⭐⭐⭐⭐ | 分层清晰，职责明确，Pregel + RuleEngine + Scorer 三件套架构成熟 |
| 代码质量 | ⭐⭐⭐ | 命名规范良好，但跨模块重复代码和有 40+ 处可替换为标准库的手动实现 |
| 规则正确性 | ⭐⭐⭐⭐ | 法律依据总体准确，存在 3 处条款引用错误需纠正 |
| 并发安全 | ⭐⭐⭐⭐ | 1 处 P0 锁缺失（`claimdrafting.RegisterAll`），其余正确 |
| 错误处理 | ⭐⭐⭐ | nil 安全策略不一致（panic vs 静默），LLM 降级路径无标记 |
| LLM 集成 | ⭐⭐ | specdrafting LLM 节点为空壳，claimdrafting 实现相对完整但容错不足 |
| 测试覆盖 | ⭐⭐⭐ | 总覆盖率 ~64.5%，规则引擎覆盖良好，但 Extension/LLM/Builder 路径大量未覆盖 |
| 文档完整 | ⭐⭐⭐⭐ | godoc 详实，法律依据标注到位，设计文档完整 |

### 模块健康度对比

| 方面 | specdrafting | claimdrafting |
|------|-------------|---------------|
| 文件数 | 15 | 17 |
| 代码行数 | 2,980 | 3,974 |
| 测试数 | 23 | 46 |
| 覆盖率 | 65.0% | 64.2% |
| LLM 实现度 | **空壳**（节点全降级） | **完整**（有 Prompt + 解析器） |
| 架构复杂度 | Pregel 图 (12 节点) | 五步法 Builder |
| 法律依据精确度 | 1 处不精确 | 3 处需纠正 |
| 并发安全 | 良好 | 1 处 P0（RegisterAll 缺锁） |

---

## 严重级别定义

| 级别 | 定义 | 行动 |
|------|------|------|
| **P0 — 致命** | 并发安全问题、数据损坏 | 立即修复 |
| **P1 — 高** | 功能缺陷、法律依据错误、架构伪实现 | 下一迭代修复 |
| **P2 — 中** | 代码重复、设计不一致、测试缺口 | 加入 Backlog |
| **P3 — 低** | 命名优化、重构残留、硬编码 | 技术债跟踪 |

---

## 问题清单

### 🔴 P0 — 致命（1 项）

#### 1. claimdrafting `RegisterAll` 缺少锁保护

**文件**: `domains/claimdrafting/rules.go:51-53`
**发现源**: 代码质量审阅

```go
func (e *RuleEngine) RegisterAll(rules ...ClaimRule) {
    e.rules = append(e.rules, rules...)  // 无锁！
}
```

与 `Register` 方法（有 `mu.Lock()/defer mu.Unlock()`）不一致。并发注册时 `append` 到 slice 存在 data race。

**对比**: specdrafting `RegisterAll` 正确持有锁（`rules.go:40`）。

**修复**: 添加锁保护：
```go
func (e *RuleEngine) RegisterAll(rules ...ClaimRule) {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.rules = append(e.rules, rules...)
}
```

---

### 🟠 P1 — 高（14 项）

#### 1. specdrafting Pregel 图节点全部忽略 LLM provider

**文件**: `domains/specdrafting/nodes.go:98-225`
**发现源**: 架构审阅 + LLM 集成审阅

所有 7 个 `draft_*Node` 函数接受 `provider agentcore.Provider` 参数但完全未使用：
```go
func draftTitleNode(provider agentcore.Provider) graph.PregelNode {
    // ... provider 参数被忽略
    builder := NewSpecBuilder(nil)
    content := builder.defaultTechField(...)
}
```

**后果**: `BuildSpecificationGraph(provider, nil, nil)` 与 `BuildSpecificationGraph(nil, nil, nil)` 输出完全相同。LLM 模式是架构上的伪实现——注释描述的双模式（LLM / 降级）从未兑现。

**修复方案 A（短期）**: 删除 `provider` 参数和 `LLMDrafter` 的 Pregel 图路径，声明本模块仅支持模板模式。

**修复方案 B（中期）**: 实现 LLM 增强：参考 claimdrafting 模式，为每个章节构建 Prompt、调用 provider、提供降级回退。

**建议**: 方案 B，按 `docs/specs/` spec-driven 流程推进。

---

#### 2. specdrafting `LLMDrafter.Draft()` 错误被静默吞掉，无降级标记

**文件**: `domains/specdrafting/drafter.go:38-56`
**发现源**: LLM 集成审阅

```go
func (d *LLMDrafter) Draft(input SpecInput) *SpecOutput {
    if d.compiled != nil {
        state, err := d.compiled.Run(...)
        if err == nil {
            if output, ok := state[StateKeyOutput].(*SpecOutput); ok && output != nil {
                return output
            }
        }
        log.Printf("specdrafting: Pregel 图执行失败: %v", err)
    }
    return d.builder.Build(input)  // 调用方无法区分的静默降级
}
```

**问题**: 调用方无法区分返回结果是来自 LLM 增强路径还是纯 Builder 降级。`extension.go:153-156` 还存在双重构建浪费（始终先执行 Builder，再被 drafter 覆盖）。

**修复**: (1) 在 `SpecOutput` 添加 `Degraded bool` 标记，降级输出 `Degraded=true`；(2) 优化双重构建：`if d := e.Drafter(); d != nil { output = d.Draft(*input) } else { output = e.builder.Build(*input) }`。

---

#### 3. claimdrafting `domain-chemical` 法律依据引用错误

**文件**: `domains/claimdrafting/rules.go:209`
**发现源**: 规则引擎审阅

```go
&domainChemicalRule{baseRule: newBaseRule("domain-chemical",
    "...", "审查指南第二部分第二章")},
```

化学领域规则应引用**审查指南第二部分第十章**（专门处理化学领域发明），而非第二部分第二章。

**对比**: specdrafting `rules_domain.go` 正确引用 "审查指南第二部分第十章"。

---

#### 4. claimdrafting `scope-equivalents-coverage` 误引等同原则侵权条款

**文件**: `domains/claimdrafting/rules.go:197`
**发现源**: 规则引擎审阅

```go
LegalBasis(): "审查指南第二部分第三章§3.4（等同原则）"
```

§3.4 等同原则是关于**侵权判定**的标准，非权利要求撰写指引。将其作为"从属权利要求为等同替换预留空间"的法律依据是误用。

**修复**: 改为 "审查指南第二部分第二章§3.3（从属权利要求）" 或删除法律依据（作为实践建议）。

---

#### 5. claimdrafting `domain-utility-model` 法律依据引用错误

**文件**: `domains/claimdrafting/rules.go:214`
**发现源**: 规则引擎审阅

```go
LegalBasis(): "专利法实施细则第2条"
```

实用新型只能有产品权利要求的依据是**专利法第 2 条第 3 款**（定义实用新型为"对产品的形状、构造或者其结合"），而非实施细则第 2 条。

**修复**: 改为 "专利法第2条第3款；审查指南第一部分第二章§6.1"。

---

#### 6. specdrafting `clarity-forbidden-words` 法律依据不精确

**文件**: `domains/specdrafting/rules.go:120`
**发现源**: 规则引擎审阅

```go
LegalBasis(): "审查指南第二部分第二章§2.1.1"
```

审查指南第二部分第二章§2.1.1 是关于说明书"清楚"的总体要求，未列出具体禁用词（"最好是""尤其是"等）。条款模糊用语受专利法第 26 条第 3 款（充分公开）约束。

**修复**: 改为 "专利法第26条第3款；审查指南第二部分第二章§2.1.1"。

---

#### 7. claimdrafting `clarity-forbidden-words` 注册与注释不一致

**文件**: `domains/claimdrafting/rules.go:124` 注册 `LegalBasis` 为 "专利法第26条第4款"；`rules_clarity.go:77` 注释为 "依据：审查指南第二部分第二章§3.2.2"
**发现源**: 规则引擎审阅

**修复**: 统一为 "专利法第26条第4款；审查指南第二部分第二章§3.2.2"。

---

#### 8. claimdrafting `parseClaimsFromLLM` 全有或全无策略

**文件**: `domains/claimdrafting/drafter.go:93-144`
**发现源**: LLM 集成审阅

任意一条权利要求解析失败就返回 nil，丢弃全部 LLM 生成结果：
```go
for _, ct := range claimTexts {
    c := parseSingleClaim(ct)
    if c == nil { return nil }  // 一条失败 → 全部丢弃
}
```

**复现场景**: LLM 在权利要求 6 上格式有小偏差（如缺少句号），1-5 全部有用结果被丢弃。

**修复**: 实现部分解析——解析成功的保留，失败的用户告警 + 用 builder 补位。

---

#### 9. claimdrafting `parseSingleClaim` 方法类型推断过于简化

**文件**: `domains/claimdrafting/drafter.go:222-250`
**发现源**: LLM 集成审阅

```go
if strings.Contains(lower, "方法") || strings.Contains(lower, "工艺") || strings.Contains(lower, "流程") {
    claimType = ClaimTypeMethod
}
```

"一种通信方法及装置"（产品权利要求）含"方法"关键词被误判为 Method；"一种工艺流程监控设备"含"工艺"被误判。

**修复**: 增加否定规则——优先检测产品关键词（装置/设备/系统/组合物），若检测到则优先判定为 Product 类型。

---

#### 10. claimdrafting `formalityNoIllustrationRule` 附图标记检测不完整

**文件**: `domains/claimdrafting/rules_formality.go:76-99`
**发现源**: 规则引擎审阅

按行检查"如图"出现且该行不包含全角左括号"（"，存在漏洞：
- 跨行写法 "如图 1\n所示" 绕过行级括号检查
- 中括号/英文括号引用 "如[图1]所示" 可绕过
- `strings.Contains(line, "图")` 可能命中"流程图"、"方框图"等非附图引用

---

#### 11. 两模块 nil 安全策略不一致

**发现源**: 代码质量审阅

| 文件 | 行为 | 问题 |
|------|------|------|
| specdrafting/scorer.go:13 `NewSpecScorer(nil)` | **panic** | |
| claimdrafting/scorer.go:15 `NewClaimScorer(nil)` | 静默允许 nil | 不一致 |
| specdrafting/extension.go:33 `NewExtension(nil)` | **panic** | |
| claimdrafting/extension.go:32 `NewExtension(nil)` | 静默允许 nil | 不一致 |

**修复**: 统一为 fail-fast（panic）或统一防御检查。

---

#### 12. 两模块 `classifyDomain` 实现不统一

**文件**: `specdrafting/nodes.go:359` vs `claimdrafting/builder.go:382`
**发现源**: 代码质量审阅

算法结构相同（关键词计分 + 特征类别加权），但关键词列表和权重值不同。算法改进需在两处同步修改。

**修复**: 提取共享实现到 `domains/patentutil/classify.go`，用参数化区分 domain-specific keywords。

---

#### 13. `uncertainWords` / `forbiddenWords` 词汇表不一致

**发现源**: 代码质量审阅

| 词汇表 | specdrafting | claimdrafting |
|--------|-------------|---------------|
| uncertainWords | 10 个词 | 14 个词 |
| forbiddenWords | 6 个词（含商业宣传用语） | 6 个词（不含商业宣传用语） |

从专利法层面，claimdrafting 的 uncertainWords 含物理维度用语（厚/薄/宽/窄），这些在说明书中同样应限制使用。specdrafting 的 forbiddenWords 含"性能卓越""市场广阔"等商业宣传用语，在权利要求中同样适用。

**修复**: 合并为并集，两模块共用。

---

#### 14. 评分器不按严重程度差异化扣分

**文件**: `specdrafting/scorer.go:59`、`claimdrafting/scorer.go:79`
**发现源**: 规则引擎审阅

```go
scores[dim] = max(0, 100.0 - float64(dimViolations) * 20.0) // 统一扣 20 分
```

error（法律红线）和 info（建议）扣分相同，评分失真。

**修复建议**:
```go
severityPenalty := map[Severity]float64{
    SeverityError:   20,
    SeverityWarning: 10,
    SeverityInfo:     5,
}
```

---

### 🟡 P2 — 中（10 项）

| # | 文件 | 发现 | 说明 |
|---|------|------|------|
| 1 | `specdrafting/nodes.go` | 重复创建 Builder 实例 | 6 个节点各自 `NewSpecBuilder(nil)`，应注入共享实例 |
| 2 | `specdrafting/extension.go:153-164` | scorer 在 Extension 路径中未被调用 | 仅有 `engine.Validate()`，遗漏 `scorer.Score()` |
| 3 | `specdrafting/graph.go:91` | `maxSteps=30` 硬编码 | 当前 12 节点足够，但应可通过 `GraphOption` 配置 |
| 4 | `specdrafting/graph.go:94-123` | 两套 API 并存 | `BuildSpecificationGraph` 三参数版与 `BuildSpecGraphWithOpts` 选项版同存，选项版无人使用 |
| 5 | `specdrafting/rules_clarity.go:65` | `clarityPFEConsistencyRule` 20 字截断假阳性 | `truncStr(p, 20)` 可能匹配无关内容，建议使用整句匹配 |
| 6 | 两端 `rules_domain.go` | 机械领域关键词覆盖不足 | specdrafting 仅 10 个机械关键词，缺少轴承/铰接/螺栓等常见术语 |
| 7 | 两端 | `Feature`/`PFETriple` 结构 100% 重复 | specdrafting 和 claimdrafting 各自独立定义了完全相同的 Feature 和 PFETriple 结构 |
| 8 | 两端 | `LLMDrafter` Provider 接口不一致 | specdrafting 直接使用 `agentcore.Provider`，claimdrafting 自定义简化 `Provider` + `ProviderAdapter` |
| 9 | 两端 | 核心类型 `Severity`/`ScoreReport`/`baseRule` 重复 | 三组类型在两端完全独立定义 |
| 10 | 两端 | LLM 降级路径零测试覆盖 | 两个模块的 `NewLLMDrafter`/`Draft()`/`DraftFromScratch()` 均无任何测试 |

---

### 🔵 P3 — 低（10 项）

| # | 文件 | 发现 |
|---|------|------|
| 1 | `specdrafting/builder.go:345` | 手动实现 `stringsTrimSpace`，应使用 `strings.TrimSpace` |
| 2 | `specdrafting/rules.go:161-191` | 手动实现 `indexOf`/`containsStr`，应使用 `strings.Index`/`strings.Contains` |
| 3 | `specdrafting/rules_structure.go:155` | 手动实现 `fmtJoin`，应使用 `strings.Join` |
| 4 | `specdrafting/nodes.go:246-247` | 字符串拼接作为 state key（`StateKeyOutput+"_violations"`），应定义为常量 |
| 5 | `claimdrafting/builder.go:95-96` | 空注释残留（`// buildIndependent 构建独立权利要求。` 孤立行） |
| 6 | `claimdrafting/drafter.go:46-49` | nil receiver 反模式——静默创建新 Builder 掩盖调用方错误 |
| 7 | `claimdrafting/rules_necessity.go:166` | `"其特征在于"` 在 `isStopTerm` 列表中重复出现 |
| 8 | `specdrafting/drafter.go:63-66` | `Provider()` 导出方法暴露内部 provider 实例，破坏封装 |
| 9 | `claimdrafting/formalityParallelClaimRule` | 循环引用检测可产生重复违规 |
| 10 | 两端 `scorer.go` | `gradeFromScore`/`parseTechDomain`/`newBaseRule` 完全相同，适合提取共享 |

---

## 跨模块代码重复量化清单

| 类别 | 重复项 | specdrafting | claimdrafting | 细化方案 |
|------|--------|-------------|---------------|---------|
| 完全相同 | `parseTechDomain` | `extension.go:213` | `extension.go:153` | 提取 `domains/patentutil/domain.go` |
| 完全相同 | `gradeFromScore` | `scorer.go:98` | `scorer.go:119` | 提取共享包 |
| 完全相同 | `Severity` 三常量 | `types.go:55` | `types.go:152` | 提取共享类型 |
| 完全相同 | `ScoreReport` 结构 | `types.go:148` | `types.go:175` | 提取共享类型 |
| 语义相同略不同 | `classifyDomain` | `nodes.go:359` | `builder.go:382` | 提取共享实现 + 参数化关键词 |
| 语义相同略不同 | `uncertainWords` | 10 个词 | 14 个词 | 合并并集 |
| 语义相同略不同 | `forbiddenWords` | 6 个词（含商业） | 6 个词（不含） | 合并并集 |
| 语义相同略不同 | `Feature` 结构 | `types.go:68` | `types.go:117` | 提取共享（6 字段完全相同） |
| 语义相同略不同 | `PFETriple` 结构 | `types.go:78` | `types.go:127` | 提取共享（4 字段完全相同） |
| 语义相同略不同 | `countKeys`/`countKeywords` | `nodes.go:412` | `builder.go:428` | 提取共享 |
| 一侧缺失 | `ChineseCharCount` | 有 | 缺失 | claimdrafting 当前不需要 |
| 一侧缺失 | `firstOr`/`safeIndex` | 有 | 缺失 | 可共享也可不共享 |

---

## LLM 集成成熟度对比

| 方面 | specdrafting | claimdrafting |
|------|-------------|---------------|
| **Provider 接口** | 直接使用 `agentcore.Provider` | 自定义简化 `Provider` 接口 |
| **Prompt 构建** | ❌ 未实现 | ✅ 结构化 Prompt（8 条撰写要求 + 格式示例） |
| **输出解析** | ❌ 未实现 | ✅ `parseClaimsFromLLM` + `parseSingleClaim` |
| **降级策略** | ⚠️ Run 图 → 失败 → Builder | ✅ Builder 确保始终有输出 → LLM 增强 → 解析失败回退 |
| **降级标记** | ❌ 无 | ❌ 无 |
| **二次验证** | ⚠️ 图内 `validateNode` | ✅ `engine.Validate()` 验证 LLM 输出 |
| **测试覆盖** | ❌ 0% | ❌ 0%（仅解析器有测试） |
| **适配器** | 不适用 | `provider_adapter.go` 桥接 `agentcore.Provider` |
| **错误处理** | ❌ 静默降级（log.Printf） | ❌ 静默降级（return fallback） |
| **实际使用状态** | **空壳**——节点全降级 | **完整但未联调**——无真实 LLM 集成测试 |

---

## 修复优先级路线图

### 迭代 1（P0 修复）

1. `claimdrafting/rules.go:51` — `RegisterAll` 添加锁保护（1 行改动）

### 迭代 2（P1 修复，按依赖关系排序）

2. `claimdrafting/rules.go:209` — `domain-chemical` 法律依据改 "审查指南第二部分第十章"
3. `claimdrafting/rules.go:214` — `domain-utility-model` 法律依据改 "专利法第2条第3款"
4. `claimdrafting/rules.go:197` — `scope-equivalents-coverage` 法律依据改 "审查指南第二部分第二章§3.3"
5. `specdrafting/rules.go:120` — `clarity-forbidden-words` 法律依据改 "专利法第26条第3款"
6. `claimdrafting/rules.go:124` — 统一法律依据与注释
7. `specdrafting/drafter.go:38-56` — 添加 `Degraded` 标记 + 优化双重构建
8. `specdrafting/extension.go:153-164` — 补充 `scorer.Score()` 调用
9. `claimdrafting/drafter.go:93-144` — `parseClaimsFromLLM` 改为部分解析
10. `claimdrafting/drafter.go:222-250` — `parseSingleClaim` 添加产品关键词否定规则
11. `claimdrafting/rules_formality.go:76-99` — 附图标记检测增强

### 迭代 3（P2 重构 + P3 清理）

12. `specdrafting/rules.go` — 添加 `strings` 导入，删除手实现方法（`indexOf`/`containsStr`/`fmtJoin`/`stringsTrimSpace`）
13. `specdrafting/nodes.go` — 共享 Builder 实例注入
14. 两端 `scorer.go` — 改为差异化扣分
15. `specdrafting/graph.go` — 简化到一套 API
16. 创建 `domains/patentutil/` — 提取共享类型和函数

### 迭代 4（测试补强）

17. 两个模块 + Extension 层单元测试
18. 两个模块 + LLMDrafter 降级路径测试
19. `claimdrafting` + 7 条未测试规则测试
20. `specdrafting` + `clarity-terminology`/`clarity-term-consistency`/`domain-electrical` 测试
21. 两端 + 评分器边界值测试（0 分 / 100 分场景）

---

## 验证配置

```bash
# 编译
go build ./domains/specdrafting/... ./domains/claimdrafting/...

# 竞态测试
go test -race -count=1 ./domains/specdrafting/... ./domains/claimdrafting/...

# 覆盖率
go test -coverprofile=coverage.out ./domains/specdrafting/... ./domains/claimdrafting/...
go tool cover -func=coverage.out | grep -E "(specdrafting|claimdrafting)"
```

---

## 附录：审阅范围与方法

### 审阅参与方

| 审阅路径 | 解读人 | 审阅文件数 | 发现数 |
|---------|--------|-----------|--------|
| 基线验证 | 自动化 | 32 | 0 |
| 路 1：架构与接口 | Claude code-reviewer | 10 | 12 |
| 路 2：规则引擎与法律依据 | Claude code-reviewer + 专利法验证 | 15+ | 12 |
| 路 3：LLM 集成与降级 | Claude code-reviewer | 6 | 10 |
| 路 4：代码质量与重复度 | Claude code-reviewer | 32 | 21 |
| **总计** | | **32** | **55**（去重后 **35** 项独立问题） |

### 基线验证结果

| 检查项 | specdrafting | claimdrafting |
|--------|-------------|---------------|
| `go build` | ✅ | ✅ |
| `go vet` | ✅ | ✅ |
| `go test -race` | ✅（23 tests, 0 race） | ✅（46 tests, 0 race） |
| 覆盖率 | 65.0% | 64.2% |
| 未覆盖文件 | `drafter.go`、`extension.go`、`builder.go` 部分 | `drafter.go`、`extension.go`、`provider_adapter.go` |
