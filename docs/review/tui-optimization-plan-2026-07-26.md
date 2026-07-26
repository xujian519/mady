# Mady TUI 模块优化执行计划

> **依据**：三份审阅报告
> - `docs/review/tui-full-audit-2026-07-26.md`（主报告，12 维度，综合 6.1/10）
> - `docs/review/tui-audit-phase1-baseline-2026-07-26.md`（阶段 1 基线）
> - `docs/review/tui-dimension3-performance-2026-07-26.md`（维度 3 性能，13 项实测）
>
> **目标**：将 28 项新发现 + 6 项 07-25 未修复 + 4 项性能优化，拆解为**可执行、可验证、风险可控**的任务清单。
>
> **原则**：
> 1. 每个 PR 控制在 3-5 个文件内（AGENTS.md "小炸弹"原则）
> 2. 先补测试，再改实现（回归保护优先）
> 3. 风险隔离：Critical 单独修复，不与重构混合
> 4. 每个任务有明确的"完成定义"（可运行命令验证）
>
> **制订日期**：2026-07-26

---

## 目录

1. [执行摘要](#一执行摘要)
2. [Sprint 0：Quick-win 基线修复（1 天）](#二sprint-0quick-win-基线修复1-天)
3. [Sprint 1：Critical 修复（2-3 天）](#三sprint-1critical-修复2-3-天)
4. [Sprint 2：测试补全 + High 修复（2 周）](#四sprint-2测试补全--high-修复2-周)
5. [Sprint 3：性能 + Medium + 架构（1-3 月）](#五sprint-3性能--medium--架构1-3-月)
6. [全局验证检查清单](#六全局验证检查清单)
7. [风险评估与回滚策略](#七风险评估与回滚策略)
8. [开放问题（需决策）](#八开放问题需决策)

---

## 一、执行摘要

### 1.1 任务总览

| Sprint | 周期 | 任务数 | 风险等级 | 核心目标 |
|--------|------|--------|----------|---------|
| **Sprint 0** | 1 天 | 8 | 🟢 低 | 机械修复，建立健康基线 |
| **Sprint 1** | 2-3 天 | 5 | 🟠 中 | 修复 5 个 Critical（阻断级） |
| **Sprint 2** | 2 周 | 13 | 🟠 中 | 测试补全 + 8 个 High 修复 |
| **Sprint 3** | 1-3 月 | 13 | 🔴 高 | 性能优化 + Medium + ParseKeys 根治 |
| **合计** | — | **39** | — | 综合评分 6.1 → 8.3+ |

### 1.2 优先级决策矩阵

```
紧急且重要 → Sprint 1（Critical）
重要不紧急 → Sprint 2（High + 测试）
紧急不重要 → Sprint 0（Quick-win，快速清理）
都不       → Sprint 3（长期改进）
```

### 1.3 关键里程碑

| 里程碑 | 完成标志 | 预期评分提升 |
|--------|---------|-------------|
| M1：Sprint 0 完成 | lint=0 / vet=0 / race=count=10 通过 | 6.1 → 6.3 |
| M2：Sprint 1 完成 | 5 Critical 全部修复 + 测试验证 | 6.3 → 7.0 |
| M3：Sprint 2 完成 | 关键路径测试覆盖率 ≥70% / High 全修复 | 7.0 → 7.8 |
| M4：Sprint 3 完成 | Overlay CoW / 错误分层 / ParseKeys 根治 | 7.8 → 8.3+ |

---

## 二、Sprint 0：Quick-win 基线修复（1 天）

**目标**：零风险机械修复，立即提升代码健康度。所有任务可独立执行，无依赖。

### 2.1 任务清单

| ID | 关联 | 标题 | 文件数 | 预估 | 风险 |
|----|------|------|--------|------|------|
| T0.1 | M-9 | 完成 itoa 去重迁移（3→0） | 3 | 20min | 🟢 |
| T0.2 | — | theme/global_test.go os.Setenv→t.Setenv | 1 | 5min | 🟢 |
| T0.3 | M-15 | LAYERS.md 依赖矩阵修正 + 计数更新 | 1 | 15min | 🟢 |
| T0.4 | H-7 | 修复 Border=BorderMuted 颜色 bug | 1 | 5min | 🟢 |
| T0.5 | — | 删除 viewport.go 空 if 块 | 1 | 5min | 🟢 |
| T0.6 | M-11 部分 | 补充 theme/stdio 高曝光导出符号 Godoc | 2-3 | 1h | 🟢 |
| T0.7 | — | 删除 internal/conv.go ITOA 包装 | 2 | 10min | 🟢 |
| T0.8 | — | 补 colorOverride 并发安全（race 风险） | 1 | 15min | 🟢 |

### 2.2 任务详细卡片

#### T0.1：完成 itoa 去重迁移

- **关联**：M-9（itoa 实际 3 处重复）
- **文件**：
  - `tui/core/sgr.go:412-422`（删除 `itoa(n int)`）
  - `tui/terminal/ansi.go:60-70`（删除 `itoa(n int64)`）
  - `tui/internal/conv.go`（保留 `ITOA`，或改用 `strconv.FormatInt`）
- **改动**：将 `sgr.go` 和 `ansi.go` 中的 `itoa(x)` 调用改为 `conv.ITOA(int64(x))` 或 `strconv.FormatInt(int64(x), 10)`
- **验证**：
  ```bash
  cd tui && go build ./... && go test -race ./core/ ./terminal/ ./internal/...
  grep -rn "func itoa" . # 应返回 0 结果
  ```
- **预估**：20 分钟
- **风险**：低（纯函数替换，签名兼容）

#### T0.2：theme/global_test.go 使用 t.Setenv

- **关联**：测试隔离性（维度 8 发现）
- **文件**：`tui/theme/global_test.go:42,48`
- **改动**：`os.Setenv("COLORFGBG", ...)` → `t.Setenv("COLORFGBG", ...)`，删除对应的 `t.Cleanup` 恢复（t.Setenv 内置）
- **验证**：
  ```bash
  cd tui && go test -race -count=3 ./theme/
  ```
- **预估**：5 分钟
- **风险**：低

#### T0.3：LAYERS.md 文档同步

- **关联**：M-15（依赖矩阵错误）+ 计数过期
- **文件**：`tui/LAYERS.md`
- **改动**：
  1. 第 12 行：`stdio` 依赖列 `Layer 0, 2` → `Layer 0, 1, 2`
  2. 第 17 行规则：`tui/stdio depends on Layer 0 and Layer 2 only` → `Layer 0, 1, and 2`
  3. 第 32 行：`90 source files (+ 40 test files)` → `102 source files (+ 50 test files)`
  4. 更新 `Last sync` 日期为 `2026-07-26`
  5. 补全 theme（7→11）、terminal（8→9）、component（35→36）、chat（14→15）的文件清单
- **验证**：
  ```bash
  cd tui && bash scripts/verify_layers.sh
  # 人工核对文件计数
  find . -name '*.go' ! -name '*_test.go' | wc -l  # 应为 102
  ```
- **预估**：15 分钟
- **风险**：低（仅文档）

#### T0.4：修复 Border=BorderMuted 颜色 bug

- **关联**：H-7（无障碍）
- **文件**：`tui/theme/semantic_theme.go:128-129`
- **改动**：`DefaultMadyDark()` 中 `Border` 和 `BorderMuted` 当前都是 `#1D3B52`，区分两色：
  - `Border`：保持 `#1D3B52` 或略亮（如 `#2A4A63`）
  - `BorderMuted`：调暗（如 `#152A3D`）
- **验证**：
  ```bash
  cd tui && go test ./theme/
  # 人工核对：两个颜色值不同
  grep -A1 "Border:" theme/semantic_theme.go | head -4
  ```
- **预估**：5 分钟
- **风险**：低（视觉变化，无逻辑影响）

#### T0.5：删除 viewport.go 空 if 块

- **关联**：维度 9 死代码
- **文件**：`tui/component/viewport.go:148-155`
- **改动**：删除空 if 块（`if cfg.Mode == ScrollbarAuto && cfg.Width < 2 { _ = cfg.Width }`）
- **验证**：
  ```bash
  cd tui && go build ./component/ && go test ./component/
  ```
- **预估**：5 分钟
- **风险**：低

#### T0.6：补充 theme/stdio 导出符号 Godoc

- **关联**：M-11（theme/stdio Godoc 严重缺失）
- **文件**：`tui/theme/style.go`、`tui/theme/palette.go`、`tui/stdio/spinner.go`、`tui/stdio/progress.go`
- **改动**：为以下高曝光导出符号补充 Godoc：
  - `theme`：`Color`、`Style`、`NewStyle`、`Fg`/`Bg`/`Bold`、`ForceColor`、`ColorEnabled`、`Palette`、`CurrentPalette`、`DetectColorMode`
  - `stdio`：`ProgressBar`、`Spinner`、`Renderer`、`LineReader`、`Confirm`、`PromptSelect`
- **验证**：
  ```bash
  cd tui && go vet ./theme/ ./stdio/
  go doc ./theme/ ./stdio/ 2>&1 | grep -E "no comment" # 应无输出
  ```
- **预估**：1 小时（~30 个符号）
- **风险**：低（纯文档）
- **建议**：可拆分为多个小 PR（theme 一组、stdio 一组）

#### T0.7：删除 internal/conv.go ITOA 包装

- **关联**：T0.1 完成后，若决定用 `strconv.FormatInt`
- **文件**：`tui/internal/conv.go`、`tui/theme/quantize.go`（更新调用）
- **改动**：删除 `ITOA` 函数，`quantize.go` 改用 `strconv.FormatInt`
- **验证**：
  ```bash
  cd tui && go build ./... && go test ./theme/ ./internal/...
  ```
- **预估**：10 分钟
- **依赖**：T0.1（若 T0.1 选择保留 ITOA 则跳过本任务）
- **风险**：低

#### T0.8：补 colorOverride 并发安全

- **关联**：维度 8 测试隔离性发现（`theme/style.go:127` 非 atomic）
- **文件**：`tui/theme/style.go:127`
- **改动**：`var colorOverride *bool` → `var colorOverride atomic.Pointer[bool]`，更新 `ForceColor`/`ColorEnabled` 的读写
- **验证**：
  ```bash
  cd tui && go test -race -count=5 ./theme/
  ```
- **预估**：15 分钟
- **风险**：低（内部实现变更，API 不变）

### 2.3 Sprint 0 验证检查清单

完成 Sprint 0 后，运行以下全部命令，所有项必须通过：

```bash
cd tui

# 1. 构建无错
- [ ] go build ./...                                    # 期望: 无输出
- [ ] go vet ./...                                      # 期望: 无输出

# 2. Lint 清零
- [ ] golangci-lint run ./...                           # 期望: 0 issues

# 3. 测试稳定（含 race）
- [ ] go test -race -count=10 ./... 2>&1 | grep FAIL    # 期望: 无输出（除已知 Kitty bug T1.1 修复前）
- [ ] go test -race -count=10 ./core/ ./theme/ ./stdio/ # 期望: 全 ok

# 4. 文档同步
- [ ] bash scripts/verify_layers.sh                     # 期望: 退出码 0
- [ ] grep "Layer 0, 1, 2" LAYERS.md                    # 期望: 命中
- [ ] grep "102 source" LAYERS.md                       # 期望: 命中

# 5. 代码质量
- [ ] grep -rn "func itoa" . | grep -v _test            # 期望: 0 结果
- [ ] grep -A1 "Border:" theme/semantic_theme.go | head -2  # 期望: Border 与 BorderMuted 值不同

# 6. Godoc 覆盖（抽样）
- [ ] go doc ./theme/ 2>&1 | grep -c "no comment"       # 期望: < 5
```

---

## 三、Sprint 1：Critical 修复（2-3 天）

**目标**：修复 5 个阻断级 Critical 问题。每个任务单独 PR，独立验证。

### 3.1 任务清单

| ID | 关联 | 标题 | 文件数 | 预估 | 风险 |
|----|------|------|--------|------|------|
| T1.1 | C-1 | 修复 Kitty 全局状态污染（测试隔离） | 1-2 | 30min | 🟠 中 |
| T1.2 | C-2 | termios 恢复错误返回 | 1 | 15min | 🟠 中 |
| T1.3 | C-3 | PanicMsg 加 captureStack（需先提升到 core） | 2-3 | 1h | 🟠 中 |
| T1.4 | C-5 | doc.go Quick Start 完整修复 | 1 | 30min | 🟢 低 |
| T1.5 | C-4 | LLM 输出 ANSI 清洗层（安全） | 2-3 | 4-6h | 🔴 高 |

### 3.2 任务详细卡片

#### T1.1：修复 Kitty 全局状态污染

- **关联**：C-1（测试确定性失败）
- **背景**：`TestProcessTerminalKittyKbdMode` 调用 `tm.SetKittyKeyboardFlags(5)` → 写全局 `kittyFlagsGlobal`，测试结束后残留，导致后续 `TestKittyAlternateKeyAndText` 失败。
- **方案 A（推荐，最小改动）**：测试侧 cleanup
  - **文件**：`tui/terminal/terminal_kitty_test.go`
  - **改动**：在 `TestProcessTerminalKittyKbdMode` 开头添加：
    ```go
    t.Cleanup(func() {
        SetKittyKeyboardFlagsFromTerminal(0) // 重置全局 flag
    })
    ```
- **方案 B（根治，长期）**：移除全局可变状态
  - **文件**：`tui/terminal/keys.go`、`tui/terminal/terminal.go`
  - **改动**：`decodeKittyU` 接受 `flags int64` 参数，而非读全局；`ParseKeys` 签名变为 `ParseKeys(data string, flags int64)`
  - **影响**：所有 `ParseKeys` 调用方需更新
- **建议**：先用方案 A 快速修复（Sprint 1），方案 B 留待 Sprint 3（架构改进）
- **验证**：
  ```bash
  cd tui && go test -race -count=10 ./terminal/ 2>&1 | grep -E "FAIL|ok"
  # 期望: 无 FAIL，连续 10 次全 ok
  ```
- **预估**：方案 A 30 分钟，方案 B 2-3 小时
- **风险**：中（方案 B 改变公共 API）

#### T1.2：termios 恢复错误返回

- **关联**：C-2（终端可能卡 raw 模式）
- **文件**：`tui/terminal/terminal.go:247`
- **改动**：
  ```go
  // 当前
  if savedValid && saved != nil {
      _ = setTermios(t.in.Fd(), saved)
  }
  return nil

  // 修改为
  if savedValid && saved != nil {
      if err := setTermios(t.in.Fd(), saved); err != nil {
          return fmt.Errorf("tui: restore termios: %w", err)
      }
  }
  return nil
  ```
- **验证**：
  ```bash
  cd tui && go build ./terminal/ && go test ./terminal/
  # 手动验证：模拟 termios 恢复失败（困难，需 mock）—— 至少确保编译和正常路径通过
  ```
- **预估**：15 分钟
- **风险**：中（调用方需处理新可能的 error；但 `Stop()` 本就返回 error，行为更正确）
- **注意**：检查所有 `Stop()` 调用方是否正确处理 error（grep `\.Stop\(\)` tui/）

#### T1.3：PanicMsg 加 captureStack

- **关联**：C-3（诊断信息缺失）
- **依赖**：需先将 `captureStack()` 从 `tui` 根包提升到 `tui/core` 包（因 `core/message.go` 不能导入 `tui` 根包，会循环依赖）
- **步骤**：
  1. **子任务 T1.3a**：在 `tui/core/` 新建 `stack.go`，移动 `captureStack` 实现
     - **文件**：新建 `tui/core/stack.go`，修改 `tui/tui_input.go`（删除本地 `captureStack`，改用 `core.CaptureStack`）
     - **改动**：`captureStack` → `core.CaptureStack`（导出）
  2. **子任务 T1.3b**：`core/message.go:189` 使用 `core.CaptureStack()`
     - **文件**：`tui/core/message.go:189`
     - **改动**：`Stack: ""` → `Stack: CaptureStack()`
- **验证**：
  ```bash
  cd tui && go build ./... && go test -race ./core/ ./tui/
  # 验证 PanicMsg 含堆栈：编写测试触发 panic，检查 Stack 非空
  ```
- **预估**：1 小时（含依赖重构）
- **风险**：中（涉及跨包移动函数）

#### T1.4：doc.go Quick Start 完整修复

- **关联**：C-5（新贡献者上手受阻）
- **文件**：`tui/doc.go:59-92`
- **改动**：修正 3 处 API 错误
  - 第 79 行：`terminal.ProcessTerminal()` → `terminal.NewProcessTerminal()`
  - 第 79 行：`tui.New(term)` → `tui.NewTUI(term)`
  - 第 82 行：`core.NewText("Hello, TUI!")` → `component.NewText("Hello, TUI!")`（并添加 import）
- **验证**：
  ```bash
  # 创建临时文件验证 doc.go 示例可编译
  mkdir -p /tmp/docgo-check && cat > /tmp/docgo-check/main.go <<'EOF'
  package main
  // 复制 doc.go 的 Quick Start 代码
  EOF
  cd tui && go build /tmp/docgo-check/main.go
  ```
- **预估**：30 分钟
- **风险**：低（纯文档修正）
- **建议**：同时添加 `Example*` 测试函数，给文档代码加编译期保护（见 T2.x）

#### T1.5：LLM 输出 ANSI 清洗层（安全关键）

- **关联**：C-4（完整注入链）
- **背景**：`hasUnrepresentableEscape`（`cellparse.go:113`）把非 SGR 序列降级为 Raw 行，`tui_render.go:156` 对 Raw 行原样写 stdout，导致 OSC 8/标题注入/DCS/APC 序列透传。
- **方案**：增加白名单清洗函数
- **文件**：
  - 新建 `tui/core/sanitize.go`（清洗逻辑）
  - 修改 `tui/core/cellparse.go`（Raw 回退时调用清洗）
  - 或修改 `tui/tui_render.go:156`（写入前清洗）
- **清洗规则**：
  - OSC 序列：只保留 `OSC 52`（剪贴板）和 `CURSOR_MARKER`；其余 OSC（0/2/8 标题/超链接）一律剥离
  - DCS/APC：非 `CURSOR_MARKER` 一律剥离
  - 非 SGR 的 CSI：剥离 `?` 私有模式（DECSET/DECRST 如 `?1049h` 切屏），只允许 SGR 和光标定位
- **核心实现**（伪代码）：
  ```go
  func SanitizeRawContent(s string) string {
      var b strings.Builder
      i := 0
      for i < len(s) {
          if s[i] == 0x1B {
              adv := SkipAnsiSeq(s, i)
              if isAllowedEscape(s[i:i+adv]) {
                  b.WriteString(s[i : i+adv])
              }
              // else: skip（剥离）
              i += adv
          } else {
              b.WriteByte(s[i])
              i++
          }
      }
      return b.String()
  }
  ```
- **验证**：
  ```bash
  cd tui && go test -race ./core/
  # 编写针对性测试：
  # - 输入含 OSC 8 file:// 链接，验证被剥离
  # - 输入含 OSC 0 标题注入，验证被剥离
  # - 输入含 DCS 设备查询，验证被剥离
  # - 输入含合法 SGR，验证保留
  ```
- **预估**：4-6 小时（含测试）
- **风险**：高（可能误伤合法图像协议如 Kitty graphics APC）
- **缓解**：白名单需包含图像协议识别（KITTY_WINDOW_ID 时允许 APC `_G`）
- **建议**：单独 PR，需 code review

### 3.3 Sprint 1 验证检查清单

```bash
cd tui

# C-1: Kitty 测试隔离
- [ ] go test -race -count=10 ./terminal/ 2>&1 | grep -c FAIL    # 期望: 0

# C-2: termios 错误返回
- [ ] grep "_ = setTermios" terminal/terminal.go                  # 期望: 0 结果
- [ ] grep "restore termios" terminal/terminal.go                 # 期望: 命中

# C-3: PanicMsg 堆栈
- [ ] grep 'Stack: ""' core/message.go                            # 期望: 0 结果
- [ ] grep "CaptureStack" core/message.go                         # 期望: 命中

# C-4: ANSI 清洗
- [ ] test -f core/sanitize.go                                    # 期望: 存在
- [ ] go test -run "TestSanitize" -v ./core/                      # 期望: PASS

# C-5: doc.go 编译
- [ ] # 复制 doc.go Quick Start 到临时 main.go，go build 成功

# 全局回归
- [ ] go test -race -count=5 ./...                                # 期望: 全 ok
- [ ] golangci-lint run ./...                                     # 期望: 0 issues
```

---

## 四、Sprint 2：测试补全 + High 修复（2 周）

**目标**：先补关键路径测试（保护后续修改），再修复 8 个 High 问题。

### 4.1 阶段 2a：测试补全（第 1 周）

**原则**：每个测试 PR 独立，先于实现修复合并。

| ID | 关联 | 标题 | 文件数 | 预估 |
|----|------|------|--------|------|
| T2.1 | M-6 | agentadapter 事件映射表驱动测试（14 类事件） | 1 | 4h |
| T2.2 | M-12 | editor_edit.go 7 个核心编辑函数测试 | 1 | 4h |
| T2.3 | 安全 | terminal/ansi.go 单元测试 + 基础 fuzz | 1 | 2h |
| T2.4 | M-14 | 补充 P0 级 Benchmark（渲染热路径） | 3-4 | 3h |
| T2.5 | — | session_selector 渲染+导航测试（最大零覆盖文件） | 1 | 3h |

#### T2.1：agentadapter 事件映射测试

- **文件**：新建 `tui/agentadapter/adapter_events_test.go`
- **改动**：表驱动测试，覆盖 14 类 agentcore 事件 → ChatEvent 转换：
  ```go
  tests := []struct{
      name string
      event agentcore.Event  // mock
      wantType reflect.Type
      wantFields map[string]interface{}
  }{
      {"AgentStart", ...},
      {"MessageDelta", ...},
      {"ToolCallStart", ...},
      // ... 共 14 类
  }
  ```
- **验证**：
  ```bash
  cd tui && go test -cover ./agentadapter/
  # 期望: 覆盖率 14.9% → 70%+
  ```
- **预估**：4 小时
- **风险**：低（仅新增测试）

#### T2.2：editor_edit.go 核心函数测试

- **文件**：新建或扩展 `tui/component/editor_edit_test.go`
- **覆盖函数**：`moveCursor`、`moveWord`、`deleteForward`、`deleteWordBackward`、`deleteWordForward`、`deleteToLineStart`、`deleteToLineEnd`
- **验证**：
  ```bash
  cd tui && go test -cover -run "TestEditor" ./component/
  # 期望: editor_edit.go 覆盖率 24.8% → 80%+
  ```
- **预估**：4 小时

#### T2.3：terminal/ansi.go 测试

- **文件**：新建 `tui/terminal/ansi_test.go`（若已存在则扩展）
- **覆盖**：所有 ANSI builder 函数（`HideCursor`/`ShowCursor`/`ClearLine`/`CursorPosition` 等）
- **附加**：基础 fuzz 测试，验证畸形输入不 panic
- **验证**：
  ```bash
  cd tui && go test -cover ./terminal/
  go test -fuzz=FuzzParseANSI -fuzztime=10s ./terminal/  # 可选
  ```
- **预估**：2 小时

#### T2.4：补充 P0 级 Benchmark

- **文件**：
  - 新建 `tui/core/render_bench_test.go`（ParseLine / SerializeRow / DiffFrame / SGR）
  - 新建 `tui/tui_render_bench_test.go`（完整帧渲染）
  - 扩展 `tui/component/markdown_bench_test.go`（Markdown 渲染）
- **参考**：维度 3 报告中的 benchmark 脚本（已验证可运行）
- **验证**：
  ```bash
  cd tui && go test -bench=. -benchmem -run=^$ ./core/ ./component/
  # 期望: 至少 8 个 benchmark 函数
  ```
- **预估**：3 小时

#### T2.5：session_selector 测试

- **文件**：新建 `tui/component/session_selector_test.go`
- **覆盖**：渲染、键盘导航、过滤、回调触发
- **验证**：
  ```bash
  cd tui && go test -cover -run "TestSessionSelector" ./component/
  ```
- **预估**：3 小时

### 4.2 阶段 2b：High 修复（第 2 周）

**前置条件**：阶段 2a 的相关测试已合并。

| ID | 关联 | 标题 | 文件数 | 预估 | 风险 |
|----|------|------|--------|------|------|
| T2.6 | H-1 | 实现 OnDebug overlay | 2-3 | 1 天 | 🟠 中 |
| T2.7 | H-3 | Editor/Input 选中色主题化 | 3 | 2h | 🟢 低 |
| T2.8 | H-4 | stdin 缓冲区容量上限 | 1 | 1h | 🟠 中 |
| T2.9 | H-5 | NewTUI 签名统一 | 2 | 1h | 🟠 中 |
| T2.10 | H-8 | verify_layers.sh 接入 CI | 2 | 1h | 🟢 低 |
| T2.11 | M-1 | session_selector 回调加 ctx | 1-2 | 3h | 🟠 中 |
| T2.12 | M-2 | stdio 持锁 I/O 修复 | 2 | 3h | 🟠 中 |
| T2.13 | M-3 | theme watcher panic 重启 | 2 | 3h | 🟠 中 |

#### T2.6：实现 OnDebug overlay

- **关联**：H-1（死代码）+ 维度 10（可观测性）
- **文件**：
  - 新建 `tui/component/debug_overlay.go`（debug 面板组件）
  - 修改 `tui/tui_input.go:262`（wire `OnDebug` 回调）
  - 修改 `tui/tui.go`（暴露 FPS/队列深度统计）
- **功能**：`ctrl+shift+d` 切换 debug overlay，显示：
  - FPS（帧率）
  - msgCh 队列深度
  - 最近 N 条事件
  - 当前 ChatApp FSM 状态
  - 内存分配统计
- **验证**：
  ```bash
  cd tui && go build ./... && go test ./tui/ ./component/
  # 手动验证：启动 TUI，按 ctrl+shift+d，确认 overlay 显示
  ```
- **预估**：1 天
- **风险**：中（新组件，需集成测试）

#### T2.7：Editor/Input 选中色主题化

- **关联**：H-3（07-25 未修复）
- **文件**：
  - `tui/component/editor_render.go:355`（删除硬编码 `48;5;33`）
  - `tui/component/input.go:246`（同上）
  - `tui/component/editor.go`/`input.go` 构造函数（注入选中色）
- **改动**：参考 `chat/chat_history_render_highlight.go:49-52` 的模式，从主题读取 `SelectedBg`
- **验证**：
  ```bash
  cd tui && go test ./component/
  grep "48;5;33" component/editor_render.go component/input.go  # 期望: 0 结果
  ```
- **预估**：2 小时
- **风险**：低

#### T2.8：stdin 缓冲区容量上限

- **关联**：H-4（安全）
- **文件**：`tui/terminal/stdin_buffer.go:340`
- **改动**：在 `consumeKeyEvents` 中增加全局容量检查：
  ```go
  const maxBufferBytes = 1 << 20 // 1MB
  if len(b.buf) > maxBufferBytes {
      b.buf = nil // 丢弃并重置（复用 SGR 鼠标的 64 字节丢弃策略）
      return 0
  }
  ```
- **验证**：
  ```bash
  cd tui && go test -run "TestStdinBuffer" ./terminal/
  # 编写测试：喂入 2MB 未终止 CSI，验证不 OOM
  ```
- **预估**：1 小时
- **风险**：中（可能丢弃合法长输入，但 1MB 远超正常使用）

#### T2.9：NewTUI 签名统一

- **关联**：H-5（API 陷阱）
- **文件**：`tui/tui.go:209,240`
- **改动**：
  1. 废弃 `NewTUI(term, ...TUIOptions)`（加 `// Deprecated` 注释）
  2. `NewTUIWithOptions` 重命名为 `NewTUI`
  3. 更新所有调用方（grep `NewTUI\b` 全代码库）
- **验证**：
  ```bash
  cd tui && go build ./...
  grep -rn "NewTUIWithOptions" . | grep -v _test  # 期望: 0 结果
  grep -rn "NewTUI(" . | grep -v "Deprecated\|_test"  # 期望: 仅新签名
  ```
- **预估**：1 小时
- **风险**：中（公共 API 变更，需更新调用方）

#### T2.10：verify_layers.sh 接入 CI

- **关联**：H-8（文档漂移无拦截）
- **文件**：
  - `.github/workflows/ci.yml`（添加 step）
  - `Makefile`（添加 `verify-layers` 目标）
- **改动**：
  ```yaml
  # ci.yml
  - name: Verify TUI layers
    run: cd tui && bash scripts/verify_layers.sh
  ```
  ```makefile
  # Makefile
  verify-layers:
	cd tui && bash scripts/verify_layers.sh
  ```
- **验证**：
  ```bash
  cd tui && bash scripts/verify_layers.sh && echo "PASS"
  make verify-layers
  ```
- **预估**：1 小时
- **风险**：低

#### T2.11：session_selector 回调加 ctx

- **关联**：M-1（goroutine 泄漏）
- **文件**：`tui/component/session_selector.go:346,398,407,420`
- **改动**：为 `SessionSelector` 增加 `ctx context.Context` 字段，4 处 `go fn(...)` 改为：
  ```go
  go func() {
      select {
      case <-s.ctx.Done():
          return
      default:
          fn(item)
      }
  }()
  ```
  或在 `Close()` 时用 `sync.WaitGroup` 等待
- **验证**：
  ```bash
  cd tui && go test -race -count=5 ./component/
  # 编写测试：创建 selector，立即 Close，确认无 goroutine 泄漏
  ```
- **预估**：3 小时
- **风险**：中（需修改构造函数签名）

#### T2.12：stdio 持锁 I/O 修复

- **关联**：M-2（并发性能）
- **文件**：`tui/stdio/renderer.go:40`、`tui/stdio/progress.go:48`
- **改动**：锁内仅组装字符串，`fmt.Fprint` 移到 `Unlock` 之后：
  ```go
  // 当前
  func (r *Renderer) WriteChunk(chunk string) {
      r.mu.Lock()
      defer r.mu.Unlock()
      // ... 组装 + flushLine（含 fmt.Fprint）
  }

  // 修改为
  func (r *Renderer) WriteChunk(chunk string) {
      r.mu.Lock()
      output := r.assembleChunkLocked(chunk)  // 锁内仅组装
      r.mu.Unlock()
      fmt.Fprint(r.writer, output)            // 锁外写入
  }
  ```
- **验证**：
  ```bash
  cd tui && go test -race ./stdio/
  ```
- **预估**：3 小时
- **风险**：中（需确保组装与写入的状态一致性）

#### T2.13：theme watcher panic 重启

- **关联**：M-3（永久死亡）
- **文件**：`tui/theme/watch.go:23`、`tui/theme/system_appearance.go:68`
- **改动**：recover 后不直接退出，而是退避重启：
  ```go
  defer func() {
      if r := recover(); r != nil {
          slog.Error("theme watcher panicked, restarting", "err", r, "stack", ...)
          time.Sleep(5 * time.Second) // 退避
          go watchLoop(...)           // 重启（需避免无限递归，用 for 循环）
      }
  }()
  ```
  或更简洁：用 `for` 循环包裹，recover 后 continue
- **验证**：
  ```bash
  cd tui && go test -race ./theme/
  # 编写测试：强制 watcher panic，验证重启
  ```
- **预估**：3 小时
- **风险**：中（退避策略需谨慎，避免无限重启）

### 4.3 Sprint 2 验证检查清单

```bash
cd tui

# 测试覆盖率提升
- [ ] go test -cover ./agentadapter/        # 期望: ≥70%（原 14.9%）
- [ ] go test -cover ./component/           # 期望: ≥65%（原 50.3%）
- [ ] go test -cover ./terminal/            # 期望: ≥65%（原 57.4%）

# 关键路径覆盖
- [ ] go test -cover -run "TestEditor" ./component/      # editor_edit.go ≥80%
- [ ] go test -cover -run "TestSessionSelector" ./component/

# Benchmark 存在
- [ ] go test -bench=. -run=^$ ./core/ 2>&1 | grep -c "Benchmark"  # 期望: ≥5
- [ ] go test -bench=. -run=^$ ./component/ 2>&1 | grep -c "Benchmark"  # 期望: ≥3

# High 修复
- [ ] grep "48;5;33" component/editor_render.go component/input.go  # 期望: 0
- [ ] grep "OnDebug" tui_input.go | grep -v "if\|//"               # 期望: 有实际调用
- [ ] grep "maxBufferBytes\|1 << 20" terminal/stdin_buffer.go       # 期望: 命中
- [ ] grep "Deprecated" tui.go | grep "NewTUI"                      # 期望: 旧签名标记

# CI
- [ ] grep "verify_layers\|verify-layers" .github/workflows/ci.yml  # 期望: 命中

# 回归
- [ ] go test -race -count=10 ./...                                  # 全 ok
- [ ] golangci-lint run ./...                                        # 0 issues
```

---

## 五、Sprint 3：性能 + Medium + 架构（1-3 月）

**目标**：长期改进，需 ADR 支持的架构决策。

### 5.1 性能优化（第 1 月）

| ID | 关联 | 标题 | 文件数 | 预估 | 风险 |
|----|------|------|--------|------|------|
| T3.1 | 性能 P1 | Overlay deep-copy → CoW | 1-2 | 1-2 天 | 🔴 高 |
| T3.2 | 性能 P2 | ParseLine 预分配 cells | 1 | 1h | 🟢 低 |
| T3.3 | 性能 P2 | Cell.Combining 指针化 | 5+ | 1 周 | 🔴 高 |
| T3.4 | 性能 P3 | SGR ParseSGR 减少分配 | 1 | 2h | 🟡 中 |

#### T3.1：Overlay CoW 优化

- **关联**：07-25 C3（未修复）+ 维度 3 P1
- **文件**：`tui/overlay.go:249-262`
- **改动**：全量 deep-copy → 仅复制被 overlay 实际修改的行：
  ```go
  modified := make([]bool, len(base))
  for _, ov := range overlays {
      origin := resolveOverlayOrigin(ov, cols, rows, w, h)
      for row := origin.row; row < origin.row+h; row++ {
          if !modified[row] {
              base[row].Cells = append([]core.Cell(nil), base[row].Cells...)
              modified[row] = true
          }
      }
  }
  ```
- **验证**：
  ```bash
  cd tui && go test -race ./...
  # 性能对比 benchmark：
  go test -bench="OverlayDeepCopy\|OverlayCompose" -benchmem ./core/ ./tui/
  # 期望: allocs 从 61 → ~20（-70%）
  ```
- **预估**：1-2 天（含测试和回归验证）
- **风险**：高（需确保 dimBackgroundRows/spliceOverlayRows 不会修改未复制的行）
- **缓解**：先添加覆盖 overlay 修改路径的测试，再做 CoW 重构

### 5.2 Medium 修复（第 1-2 月）

| ID | 关联 | 标题 | 预估 | 风险 |
|----|------|------|------|------|
| T3.5 | M-7 | 统一三套终端检测系统 | 1 天 | 🟠 中 |
| T3.6 | M-8 | Sixel 实现或文档修正 | 2h-1 周 | 🟢-🔴 |
| T3.7 | M-10 | 置信度条渲染器去重 | 3h | 🟢 低 |
| T3.8 | M-5 | ChatEvent 三重标识收敛 | 1 天 | 🟠 中 |
| T3.9 | — | 拆分 AppHost 接口（ISP） | 3h | 🟡 中 |
| T3.10 | — | 补 8 个零覆盖整文件的渲染快照测试 | 2-3 天 | 🟢 低 |

#### T3.5：统一终端检测系统

- **关联**：M-7（三套检测断裂）
- **文件**：`tui/theme/color_resolve.go`、`tui/terminal/terminal.go:396`
- **改动**：
  1. `theme.DetectColorMode()` 委托到 `terminal.CurrentTerminalContext().HasTrueColor()`
  2. 删除 `TerminalSupportsKittyKeyboard()`（重复实现），改用 `TerminalContext.SupportsKittyKeyboard`
- **验证**：
  ```bash
  cd tui && go test -race ./theme/ ./terminal/
  ```
- **风险**：中（需确保所有调用方更新）

### 5.3 架构改进（第 2-3 月，需 ADR）

| ID | 关联 | 标题 | 预估 | 风险 |
|----|------|------|------|------|
| T3.11 | M-4 | ~~消除 tui→chat 跨层依赖~~ **→ 已取消，改为 T3.16 记录妥协** | — | — |
| T3.12 | — | ~~集成 pkg/i18n~~ **→ 已取消（用户决策：暂时不需要）** | — | — |
| T3.13 | H-2 | 引入错误类型分层 | 1 周 | 🟠 中 |
| T3.14 | — | 新增 high-contrast + 色盲主题 | 3 天 | 🟢 低 |
| T3.15 | — | 提升 LAYERS.md Key Decisions 为正式 ADR | 1 天 | 🟢 低 |
| **T3.16** | **M-4/Q4** | **LAYERS.md 增加"已知架构妥协"章节（tui→chat 依赖）** | **1h** | **🟢 低** |
| **T3.17** | **C-1/Q1** | **ParseKeys 移除全局状态（根治，方案 B）** | **2-3h** | **🟠 中** |

> **决策记录**：
> - **Q3 i18n**：用户决策"暂时不需要"。T3.12 整体取消。后续若启动需重新评估。
> - **Q4 架构反转**：采纳"不启动"建议。T3.11 取消，改为 T3.16（文档记录妥协）。
> - **Q1 Kitty 根治**：Sprint 1 已用方案 A 止血，T3.17 在 Sprint 3 做方案 B 根治，与 T3.5（统一终端检测）合并为一个 PR。

#### T3.16：LAYERS.md 增加"已知架构妥协"章节

- **关联**：M-4 / Q4 决策（不启动架构反转）
- **文件**：`tui/LAYERS.md`
- **改动**：在 "Key Design Decisions" 章节后新增一节：
  ```markdown
  ### Known Architectural Compromises

  #### tui (L3) → chat (L5) Dependency

  The root `tui` package imports `tui/chat` (L5) via `chat_bridge.go` to
  provide the `NewChatApp` convenience constructor. This is an upward
  dependency (L3 → L5) that technically violates the strict layering rule.

  **Why it exists**: `NewChatApp` creates both a `TUI` engine and a
  `ChatApp` wired together via the `tuiAppHost` adapter. Moving this to
  `chat` would break the public API (`tui.NewChatApp` is used by
  `cmd/mady` and `example/`).

  **Why we accept it**: The dependency is isolated to a single file
  (`chat_bridge.go`) with a well-designed adapter pattern. The cycle is
  broken at the interface level (`chat.AppHost`). The cost of reversing
  it (1-2 weeks, breaking API change) outweighs the benefit for an
  internal-only module.

  **When to revisit**: If TUI is ever extracted as a standalone library
  for external consumption, this dependency must be reversed first.
  ```
- **验证**：
  ```bash
  grep "Known Architectural Compromises" tui/LAYERS.md  # 期望: 命中
  bash tui/scripts/verify_layers.sh                    # 期望: 仍通过
  ```
- **预估**：1 小时
- **风险**：低（纯文档）

### 5.4 Sprint 3 验证检查清单

```bash
cd tui

# 性能
- [ ] go test -bench="Overlay" -benchmem ./core/ | tee after.txt
- [ ] # 对比 baseline：allocs 减少 ≥70%
- [ ] go test -bench="ParseLine" -benchmem ./core/ | tee after.txt
- [ ] # 对比 baseline：allocs 减少 ≥20%

# 终端检测统一
- [ ] grep "DetectColorMode" theme/color_resolve.go | grep "CurrentTerminalContext"  # 命中

# Q1 根治：ParseKeys 无全局状态
- [ ] grep "kittyFlagsGlobal" terminal/keys.go | grep -v "_test\|//"  # 0 结果（已移除）
- [ ] grep "func ParseKeys" terminal/keys.go | grep "flags"          # 新签名含 flags 参数

# Q4 妥协记录
- [ ] grep "Known Architectural Compromises" tui/LAYERS.md           # 命中

# 测试覆盖
- [ ] go test -cover ./... 2>&1 | grep "coverage" | awk '{print}'  # 总体 ≥70%

# 全局
- [ ] make verify  # lint + build + test-race 全过
```

---

## 六、全局验证检查清单

### 6.1 每个 PR 必须通过的检查（CI 门禁）

```bash
cd tui

# 必须全过
- [ ] go build ./...                          # 构建无错
- [ ] go vet ./...                            # vet 无告警
- [ ] golangci-lint run ./...                 # lint 0 issues
- [ ] go test -race -count=3 ./...            # 测试全过（含 race）
- [ ] bash scripts/verify_layers.sh           # 分层依赖校验
```

### 6.2 里程碑验收检查

#### M1（Sprint 0 完成）

```bash
- [ ] 上述 CI 门禁全过
- [ ] grep -rn "func itoa" tui/ | grep -v _test               # 0 结果
- [ ] grep "Layer 0, 1, 2" tui/LAYERS.md                      # 命中
- [ ] go test -race -count=10 ./tui/... 2>&1 | grep FAIL      # 仅可能的 Kitty（T1.1 前）
```

#### M2（Sprint 1 完成）

```bash
- [ ] go test -race -count=10 ./tui/terminal/ 2>&1 | grep FAIL  # 0（Kitty 已修）
- [ ] grep "_ = setTermios" tui/terminal/terminal.go            # 0
- [ ] grep 'Stack: ""' tui/core/message.go                      # 0
- [ ] test -f tui/core/sanitize.go                              # 存在
- [ ] # doc.go Quick Start 可编译
```

#### M3（Sprint 2 完成）

```bash
- [ ] go test -cover ./tui/agentadapter/ | grep coverage        # ≥70%
- [ ] go test -cover ./tui/component/ | grep coverage           # ≥65%
- [ ] go test -bench=. -run=^$ ./tui/core/ 2>&1 | grep -c Bench # ≥5
- [ ] grep "48;5;33" tui/component/editor_render.go             # 0
- [ ] grep "verify.layers" .github/workflows/ci.yml             # 命中
```

#### M4（Sprint 3 完成）

```bash
- [ ] go test -bench="Overlay" -benchmem ./tui/core/            # allocs < 20
- [ ] grep "kittyFlagsGlobal" tui/terminal/keys.go | grep -v "_test\|//"  # 0（已根治）
- [ ] grep "Known Architectural Compromises" tui/LAYERS.md      # 命中
- [ ] go test -cover ./tui/... 2>&1 | grep "coverage"           # 总体 ≥70%
```

### 6.3 回归测试脚本（一键验证）

将以下保存为 `tui/scripts/full-check.sh`：

```bash
#!/bin/bash
set -e
cd "$(dirname "$0")/.."

echo "=== 1. Build ==="
go build ./...

echo "=== 2. Vet ==="
go vet ./...

echo "=== 3. Lint ==="
golangci-lint run ./...

echo "=== 4. Race tests (count=5) ==="
go test -race -count=5 ./...

echo "=== 5. Layer verification ==="
bash scripts/verify_layers.sh

echo "=== 6. Coverage ==="
go test -coverprofile=/tmp/tui-cov.out ./...
go tool cover -func=/tmp/tui-cov.out | tail -1

echo "=== 7. Benchmarks (quick) ==="
go test -bench=. -benchmem -benchtime=100ms -run=^$ ./core/ ./component/ ./chat/ 2>&1 | grep Benchmark || true

echo ""
echo "✅ All checks passed!"
```

---

## 七、风险评估与回滚策略

### 7.1 高风险任务清单

| 任务 | 风险 | 失败影响 | 回滚策略 |
|------|------|---------|---------|
| T1.5 ANSI 清洗 | 误伤合法图像协议 | 图像降级为 HalfBlock | 环境变量 `MADY_TUI_RAW_PASSTHROUGH=1` 关闭清洗 |
| T3.1 Overlay CoW | dim/splice 修改未复制行 | 渲染错乱 | git revert（有测试保护） |
| T3.3 Cell.Combining 指针化 | 全代码库访问路径 | 编译失败 | 分阶段迁移，保留旧 API |
| T3.17 ParseKeys 签名变更 | 公共 API 破坏性变更 | 调用方编译失败 | 保留旧签名 + Deprecated 过渡 |
| T2.13 watcher 重启 | 无限重启循环 | CPU 100% | 退避上限 + 熔断 |

### 7.2 通用回滚原则

1. **每个任务独立 PR**：便于单独 revert
2. **高风险任务前先补测试**：测试失败即阻止合并
3. **破坏性 API 变更**：保留旧 API + `// Deprecated`，至少一个版本后才删除
4. **数据库/文件格式变更**：不适用（TUI 无持久化）

### 7.3 灰度策略（适用于高风险任务）

对于 T1.5（ANSI 清洗）等安全相关变更：
1. 添加环境变量 `MADY_TUI_RAW_PASSTHROUGH=1`（默认关闭=激进清洗开启，设为 1 则跳过清洗）
2. 先合入清洗逻辑但默认关闭清洗，观察 1 周
3. 确认无副作用后开启默认清洗（即移除默认关闭的临时逻辑）

---

## 八、决策记录（已确认）

> **状态**：全部已决策（2026-07-26）。以下记录最终选择及其依据。

### Q1：C-1 Kitty 全局状态修复方案 → **先 A 后 B**

- **决策**：Sprint 1 用方案 A（测试 cleanup，30min）止血；Sprint 3 用方案 B（移除全局状态）根治
- **依据**：C-1 是阻断级必须快速修复让 CI 稳定；根治与 T3.5（统一终端检测）合并为一个 PR，减少对调用方的多次冲击
- **影响任务**：T1.1（方案 A）+ T3.17（方案 B，新增）

### Q2：C-4 ANSI 清洗的范围 → **方案 B 激进白名单 + 环境变量灰度**

- **决策**：剥离所有非 SGR 序列，仅白名单放行；图像协议默认降级为 HalfBlock；提供 `MADY_TUI_RAW_PASSTHROUGH=1` 关闭清洗
- **依据**：TUI 处理 LLM 输出，prompt injection 可诱导模型输出任意字节；法律/专利场景 OSC 8 `file://` 链接泄露风险不可接受；图像降级为 HalfBlock 仍可显示（非功能丢失）
- **影响任务**：T1.5（按方案 B 实现 + 环境变量开关）

### Q3：i18n 集成 → **暂时不需要（已取消）**

- **决策**：移除 T3.12 全部 i18n 相关任务
- **依据**：用户决策。当前优先级在安全/正确性/可观测性，i18n 是体验改进非功能缺陷
- **影响任务**：T3.12a/b/c 全部取消；后续若启动需重新评估范围

### Q4：架构反转 T3.11 → **不启动，改为文档记录妥协**

- **决策**：不启动 tui→chat 依赖反转；新增 T3.16 在 LAYERS.md 记录"已知架构妥协"
- **依据**：实际影响面小（仅 chat_bridge.go 1 文件）；机会成本高（1-2 周相当于 Sprint 2 全部 High 工时）；模块瓶颈在测试/安全/可观测性不在架构分层
- **影响任务**：T3.11 取消；新增 T3.16（文档记录，1h）

### Q5：任务执行方式 → **混合模式**

- **决策**：机械任务（Sprint 0 / 测试补全 / Benchmark）用 AI 执行 + 人工 review；关键任务（Critical / 架构 / 性能优化）人工执行
- **执行流程**：AI 产出 PR → 人工 review + `make verify` → 合并 → 下一个任务
- **分工表**：

| 任务类型 | 执行方 | 理由 |
|---------|--------|------|
| Sprint 0 Quick-win（T0.1-T0.8） | 🤖 AI | 纯机械（删函数/改文档/补 Godoc） |
| 测试补全（T2.1-T2.5） | 🤖 AI | 表驱动测试是模式化工作 |
| Benchmark（T2.4） | 🤖 AI | 已有验证脚本 |
| Critical 修复（T1.1-T1.5） | 👤 人工 | 安全/正确性关键，尤其 T1.5 ANSI 清洗 |
| 架构/性能（T3.1, T3.3, T3.5, T3.17） | 👤 人工 | 需设计判断，AI 易引入 subtle bug |
| i18n 迁移 | — | 已取消 |

---

## 附录：任务依赖关系图

```
Sprint 0（独立，无依赖）
├── T0.1 ──→ T0.7（可选）
├── T0.2, T0.3, T0.4, T0.5, T0.6, T0.8（完全独立）

Sprint 1
├── T1.1（独立，方案 A 止血）
├── T1.2（独立）
├── T1.3a ──→ T1.3b（captureStack 提升后再用）
├── T1.4（独立）
├── T1.5（独立，建议在 T2.3 ansi.go 测试之后）

Sprint 2a（测试补全，独立于 2b）
├── T2.1, T2.2, T2.3, T2.4, T2.5（相互独立）

Sprint 2b（High 修复，依赖 2a 测试）
├── T2.6（依赖 T2.4 benchmark 数据）
├── T2.7（建议先有 T2.2 editor 测试）
├── T2.8（建议先有 T2.3 terminal 测试）
├── T2.9, T2.10（独立）
├── T2.11（建议先有 T2.5 session_selector 测试）
├── T2.12, T2.13（独立）

Sprint 3
├── T3.1（建议先有 T2.4 overlay benchmark）
├── T3.2, T3.3, T3.4（独立）
├── T3.5 + T3.17（合并为一个 PR：统一终端检测 + ParseKeys 根治）
├── T3.16（独立，纯文档）
├── T3.13, T3.14, T3.15（独立）

已取消
├── T3.11（架构反转 → 改为 T3.16 文档记录）
├── T3.12a/b/c（i18n → 用户决策暂时不需要）
```

---

> **计划制订**：2026-07-26
> **审阅依据**：`tui-full-audit-2026-07-26.md` + `tui-dimension3-performance-2026-07-26.md` + `tui-audit-phase1-baseline-2026-07-26.md`
> **执行前**：请先决策第八节的 5 个开放问题
