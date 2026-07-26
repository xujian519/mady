# TUI 模块优化 — 会话交接文档

> **创建时间**：2026-07-26
> **当前分支**：`main`
> **最新提交**：`7873b19`（Sprint 2 batch 2）
> **工作区状态**：干净（无未提交改动）
> **依据文档**：`docs/review/tui-optimization-plan-2026-07-26.md`（1086 行完整计划）

---

## 一、已完成工作总览

### 1.1 四次提交（按时间顺序）

| 提交 | Sprint | 任务 | 文件数 | 行数变化 |
|------|--------|------|--------|---------|
| `1de3356` | Sprint 0 | T0.1–T0.8（8 项 Quick-win） | 14 | +142/-91 |
| `1f1d860` | Sprint 1 | T1.1–T1.5（5 项 Critical） | 16 | +1967/-27 |
| `abe880e` | Sprint 2 batch 1 | T2.3, T2.7–T2.10（5 项） | 8 | +225/-6 |
| `7873b19` | Sprint 2 batch 2 | T2.1, T2.2, T2.11–T2.13（5 项） | 8 | +1840/-101 |

**合计**：24/25 项任务完成，31 个文件，+4032/-134 行。

### 1.2 已修复问题清单

**Sprint 0（Quick-win 基线）**：
- T0.1+T0.7：3 处 `itoa` 重复实现 → 统一 `strconv.FormatInt`，删除 `internal/conv.go`
- T0.2：`theme/global_test.go` 的 `os.Setenv` → `t.Setenv`
- T0.3：`LAYERS.md` 依赖矩阵修正（stdio 依赖 Layer 0,1,2）+ 文件计数同步
- T0.4：`Border`/`BorderMuted` 同色 `#1D3B52` → 拆分为 `#2A4A63`/`#152A3D`
- T0.5：删除 `viewport.go` 空 if 死代码
- T0.6：~25 个高曝光导出符号补充 Godoc（theme/stdio）
- T0.8：`colorOverride *bool` → `atomic.Pointer[bool]`（并发安全）

**Sprint 1（Critical 阻断级）**：
- T1.1 (C-1)：Kitty 全局状态污染 — 测试 cleanup 重置 `kittyFlagsGlobal`。**`-race -count=10` 从间歇失败变为连续全绿**
- T1.2 (C-2)：termios 恢复错误 `_ = setTermios(...)` → 返回 `fmt.Errorf`
- T1.3 (C-3)：PanicMsg.Stack 从 `""` → `core.CaptureStack()`（提升到 `tui/core/stack.go`）
- T1.4 (C-5)：doc.go Quick Start 3 处 API 修正
- T1.5 (C-4)：**ANSI 注入清洗层** — `core/sanitize.go` 白名单策略（仅 SGR + CursorMarker），双层防御（SerializeRow + 差分写入点），13 个注入向量测试

**Sprint 2（测试补全 + High/Medium）**：
- T2.1：agentadapter 事件映射测试 — 覆盖率 **14.9% → 100%**
- T2.2：editor 核心编辑函数测试 — 7 个函数 **0% → 91-100%**
- T2.3：terminal/ansi.go 单元测试 + fuzz（380 万次无 panic）
- T2.7 (H-3)：Editor/Input 选中色主题化，删除硬编码 `48;5;33`
- T2.8 (H-4)：stdin 缓冲区 1MiB 容量上限
- T2.9 (H-5)：`NewTUIWithOptions` 标记 Deprecated
- T2.10 (H-8)：`verify_layers.sh` 接入 CI + Makefile
- T2.11 (M-1)：session_selector 回调加 ctx + Close()，消除 goroutine 泄漏
- T2.12 (M-2)：stdio 持锁 I/O 修复 — flushLine/render 返回字符串，写入移到锁外
- T2.13 (M-3)：theme watcher panic 后 5s 退避重启

### 1.3 当前质量基线

```
golangci-lint run ./tui/...           → 0 issues
go build ./... (tui + root)            → 通过
go vet ./... (tui)                     → 通过
go test -race -count=1 ./... (tui)     → 11 包全绿
go test -race -count=10 ./terminal/    → 连续 10 次全绿（C-1 已修）
bash tui/scripts/verify_layers.sh      → ✅ 102 文件一致
agentadapter 覆盖率                     → 100%
editor_edit.go 核心函数覆盖率           → 91-100%
```

---

## 二、剩余任务

### 2.1 Sprint 2 未完成（1 项）

#### T2.6：实现 OnDebug overlay（H-1 死代码 + 可观测性）

- **预估**：1 天
- **风险**：中（新组件 + 集成）
- **详细卡片**：见 `docs/review/tui-optimization-plan-2026-07-26.md` 第 528-548 行

**需求**：
- 新建 `tui/component/debug_overlay.go`（debug 面板组件）
- 修改 `tui/tui_input.go:262`（wire `OnDebug` 回调，当前是空 if 块）
- 修改 `tui/tui.go`（暴露 FPS/队列深度统计）

**功能**：`ctrl+shift+d` 切换 debug overlay，显示：
- FPS（帧率）
- `msgCh` 队列深度
- 最近 N 条事件
- 当前 ChatApp FSM 状态
- 内存分配统计

**关键上下文**：
- `tui_input.go` 中已有 `OnDebug` 的空回调位置（grep `OnDebug` 定位）
- 参考 `component/review_gate.go` / `component/command_center.go` 的 overlay 组件模式
- overlay 通过 `tui.PushOverlay()` 挂载

### 2.2 Sprint 3（13 项，1-3 月长期）

**完整卡片见** `docs/review/tui-optimization-plan-2026-07-26.md` 第 730 行起。

| ID | 标题 | 预估 | 优先级 |
|----|------|------|--------|
| T3.1 | Overlay CoW（深拷贝优化，25μs/248KB → 接近零） | 3-5 天 | 高 |
| T3.2 | 错误分层（terminal/network/logical 三层） | 2-3 天 | 中 |
| T3.3 | 渲染预算监控（frame > 16ms 告警） | 1 天 | 中 |
| T3.4 | Markdown 块缓存持久化 | 2 天 | 中 |
| T3.5+T3.17 | ParseKeys 根治（移除全局 `kittyFlagsGlobal`） | 2-3 天 | 高 |
| T3.6 | Cell 池化（减少 GC 压力） | 3 天 | 低 |
| T3.7 | SGR 状态压缩（连续 Reset 合并） | 1 天 | 低 |
| T3.8 | 会话恢复（断点续传） | 1 周 | 中 |
| T3.9 | 多窗口/分屏支持 | 1-2 周 | 低 |
| T3.10 | 触摸/手势支持 | 3 天 | 低 |
| T3.13 | 主题热重载增强 | 1 天 | 低 |
| T3.14 | 可访问性（screen reader） | 1 周 | 低 |
| T3.15 | 国际化基础设施（用户已决定暂不做 i18n，此任务取消） | — | 取消 |
| T3.16 | LAYERS.md "已知架构妥协"文档段 | 1h | 低 |

**Sprint 3 重点关注**：
- **T3.1 Overlay CoW** 是最大性能债务（维度 3 报告：overlay 激活时每帧 25μs/248KB 深拷贝）
- **T3.5+T3.17 ParseKeys 根治** 是 C-1 的最终方案（当前 Sprint 1 用测试 cleanup 临时修复）
- **T3.16** 是 1 小时的文档任务，可随时穿插

---

## 三、关键文件索引

### 3.1 审阅报告（只读参考）

| 文件 | 用途 |
|------|------|
| `docs/review/tui-full-audit-2026-07-26.md` | 主报告（12 维度，综合 6.1/10，28 项发现） |
| `docs/review/tui-audit-phase1-baseline-2026-07-26.md` | 阶段 1 基线（07-25 修复追踪） |
| `docs/review/tui-dimension3-performance-2026-07-26.md` | 性能维度（13 项 benchmark 实测数据） |
| `docs/review/tui-optimization-plan-2026-07-26.md` | **优化计划（1086 行，任务卡片权威源）** |

### 3.2 本次改动涉及的关键文件

| 文件 | 改动内容 |
|------|---------|
| `tui/core/sanitize.go` | **新增**：ANSI 清洗层（C-4 修复） |
| `tui/core/sanitize_test.go` | **新增**：13 个注入向量测试 |
| `tui/core/stack.go` | **新增**：CaptureStack（C-3 修复） |
| `tui/tui_render.go:156-160` | 差分渲染 RawContent 清洗调用点 |
| `tui/core/cellrender.go:23-26` | SerializeRow 全帧渲染清洗调用点 |
| `tui/terminal/terminal_kitty_test.go` | Kitty 全局状态 cleanup（C-1 修复） |
| `tui/terminal/terminal.go:250` | termios 错误返回（C-2 修复） |
| `tui/terminal/stdin_buffer.go:126-140` | 1MiB 缓冲区上限（H-4 修复） |
| `tui/component/session_selector.go` | ctx + Close() + goCallback（M-1 修复） |
| `tui/stdio/renderer.go` | 持锁 I/O 修复（M-2 修复） |
| `tui/stdio/progress.go` | 持锁 I/O 修复（M-2 修复） |
| `tui/theme/watch.go` | panic 退避重启（M-3 修复） |
| `tui/theme/system_appearance.go` | panic 退避重启（M-3 修复） |

### 3.3 新会话快速定位命令

```bash
# 查看完整优化计划
cat docs/review/tui-optimization-plan-2026-07-26.md

# 查看本次所有改动
git log --oneline 1de3356^..HEAD

# 查看某个任务的代码（示例：C-4 清洗层）
cat tui/core/sanitize.go
cat tui/core/sanitize_test.go

# 验证当前基线
cd tui && go build ./... && go vet ./... && go test -race -count=1 ./...
cd tui && golangci-lint run ./...
```

---

## 四、关键决策记录（已确认）

这 5 个决策在前一会话中已与用户确认，新会话无需重新询问：

| # | 决策点 | 选择 | 理由 |
|---|--------|------|------|
| Q1 | Overlay CoW vs immutable snapshot | CoW（写时复制） | 平衡性能与改动量 |
| Q2 | 错误分层模型 | 三层（terminal/network/logical） | 已有隐式分层，显式化即可 |
| Q3 | ParseKeys 全局状态 | Sprint 3 根治（T3.5+T3.17） | Sprint 1 用测试 cleanup 临时修复 |
| Q4 | 执行模式 | 混合（机械任务 AI / 复杂任务人工审阅） | 效率与安全平衡 |
| Q5 | i18n 集成 | **暂不需要**（T3.15 取消） | 用户明确决定 |

---

## 五、新会话起步指南

### 5.1 第一步：读取上下文

```
请读取以下文件了解当前状态：
1. docs/review/tui-session-handoff-2026-07-26.md（本文件）
2. docs/review/tui-optimization-plan-2026-07-26.md（完整计划，重点看剩余任务）
```

### 5.2 第二步：确认基线

```bash
cd tui && go build ./... && go vet ./... && go test -race -count=1 ./... && golangci-lint run ./...
```

预期：全部通过，0 issues。

### 5.3 第三步：选择继续方向

**选项 A：完成 T2.6（OnDebug overlay）**
- 工作量：1 天
- 收益：补全 Sprint 2，消除 H-1 死代码
- 详见计划文档第 528-548 行

**选项 B：开始 Sprint 3 性能优化**
- 从 T3.1（Overlay CoW）开始 — 最大性能债务
- 或从 T3.16（LAYERS.md 妥协文档，1h）开始 — 快速热身

**选项 C：运行完整 Benchmark 建立性能基线**
- 参考 `docs/review/tui-dimension3-performance-2026-07-26.md` 的 benchmark 方法
- 为 Sprint 3 的优化效果建立对比基准

### 5.4 注意事项

1. **多模块结构**：tui 是独立子模块（`tui/go.mod`），改动后需单独 `cd tui && go build/test`
2. **提交规范**：Conventional Commits（`fix:`/`feat:`/`refactor:`/`docs:`），pre-commit hooks 会校验
3. **敏感路径**：tui 模块**不在**敏感路径清单中（已确认），但改动 `tui/terminal/terminal.go` 等仍需谨慎
4. **变更即记录**：每次功能改动后在 `docs/decisions/AI_CHANGELOG.md` 追加记录
5. **任务粒度**：单次改动 3-5 个文件（AGENTS.md "小炸弹"原则）
6. **测试先行**：复杂改动先补测试再改实现
7. **中文回答 + 英文代码**：遵循 AGENTS.md 规范

---

## 六、技术备注

### 6.1 C-1 Kitty 修复说明

当前用**方案 A**（测试 cleanup）临时修复。根治方案（**T3.5+T3.17**）需要：
- 移除 `tui/terminal/keys.go:523` 的 `var kittyFlagsGlobal int64`
- `decodeKittyU` 接受 `flags int64` 参数
- `ParseKeys` 签名变为 `ParseKeys(data string, flags int64)`
- 更新所有 `ParseKeys` 调用方

### 6.2 C-4 清洗层说明

`SanitizeRawContent` 使用**严格白名单**（不是黑名单）：
- **允许**：SGR 序列（`ESC[...m`）+ CursorMarker（`ESC_pi:c BEL`）
- **剥离**：OSC/DCS/APC/PM + 非 SGR 的 CSI（含 DEC 私有模式 `?1049h` 等）

**双层防御**：
1. `SerializeRow`（全帧渲染路径）
2. `tui_render.go:160`（差分渲染 RawContent 路径）

**注意**：Kitty graphics APC（`ESC_G...`）也被剥离。如果未来需要支持图像协议，需要在 `isAllowedRawEscape` 中添加白名单条目（参考计划文档 T1.5 的缓解策略）。

### 6.3 子代理使用经验

T2.1 和 T2.2 通过并行子代理完成，效果很好：
- agentadapter 测试：子代理 3.5 分钟完成，覆盖率 14.9% → 100%
- editor 测试：子代理 3 分钟完成，7 个函数 0% → 91-100%
- **经验**：测试补全任务适合委派子代理（边界清晰、不改实现代码、有明确的覆盖率目标）

---

*文档结束。如有疑问，先读 `docs/review/tui-optimization-plan-2026-07-26.md` 的对应任务卡片。*
