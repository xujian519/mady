# Phase 3 审阅：R12 TUI 渲染 + R13 PDF Chrome 渲染 — 2026-07-25

> Phase 3 子审阅｜依据：`Mady 全面审阅计划 v1.0` ｜执行者：AI（Grok）｜Human Owner：[NEEDS CLARIFICATION]
> 审阅窗口：2026-07-24 新增/修改渲染代码

## 摘要

### R12 TUI 渲染（3 条）

1. **✅ UTF-8 截断安全——历史 #P1 教训已被规避**。`tui/core/width.go:229-248` 的 `TruncateToWidth` 用 `utf8.DecodeRuneInString` 逐 rune 推进，`i += size`，绝不按字节切割。这正是 `context_engine_tiered.go` 当年按字节截断导致中文 UTF-8 无效字符串（agent-runtime #P1）的反面解法。tui_render.go 终极截断与 flex.go 安全网都走这条 rune-aware 路径，无复发风险。
2. **⚠️ delta 去重"第三模式"（re-emitted suffix）无测试覆盖**。`chat_history.go:517` 的 `strings.HasSuffix(current, delta)` 分支在测试文件中**无对应用例**。CHANGELOG 声称三模式均有测试，实际模式③是回归盲点。
3. **ℹ️ `t.lastCursor` 在 renderFrame 中未持锁访问**。安全性依赖"单一渲染 goroutine 由 tickCh 串行化驱动"隐式契约。当前正确，但契约脆弱。

### R13 PDF Chrome 渲染（3 条）

1. **🔴 HTML 注入面未设防（最重要）**。链路：`tools.go:113` Variables（来自 LLM 工具调用）→ Markdown 模板插值 → `gmhtml.WithUnsafe()`（renderer_html.go:37，**原始 HTML 透传不转义**）→ `page.SetDocumentContent` 注入 about:blank。vars 全程无 sanitize/escape。
2. **⚠️ `--no-sandbox` 无条件启用 + 风格不一致**。renderer 用字符串式 `chromedp.Flag("no-sandbox", true)`，同项目 browser_supervisor.go 用类型化 `chromedp.NoSandbox`。
3. **✅ 子进程生命周期与端口治理良好**。context.WithTimeout + 三层 defer cancel；DefaultExecAllocatorOptions 默认 remote-debugging-port=0（OS 随机）+ 127.0.0.1（仅本机），**无端口冲突/未授权访问风险**；PDFAutoRenderer 用 sync.Once 惰性探测，CI 无 Chrome 自动降级 gopdf。

## 1. 审阅范围

| 模块 | 文件 | 行数 | 角色 |
|------|------|------|------|
| R12 | `tui/tui_render.go` | 240 | 终极截断 + 帧合成 |
| R12 | `tui/layout/flex.go` | 512 | 安全网加固 |
| R12 | `tui/chat/chat_history.go` | 622 | delta 去重三模式 |
| R12辅 | `tui/core/width.go` | - | UTF-8 安全性证据 |
| R13 | `domains/doctmpl/renderer_pdf_chrome.go` | 173 | chromedp 子进程调用 |
| R13辅 | `domains/doctmpl/renderer_pdf_chrome_test.go` | 84 | Chrome 测试 |
| R13辅 | `domains/doctmpl/{renderer_html,renderer_registry,format,tools}.go` | - | 接口契约/注入面 |

## 2. 审阅维度执行情况（5 Lens）

| 维度 | R12 TUI | R13 PDF Chrome |
|------|---------|----------------|
| Lens-1 Go 编码 | ✅ 截断 rune-aware；⚠️ lastCursor 锁缺失（依赖单 goroutine 契约）；✅ applyDeltaLocked 锁内调用 | ✅ defer cancel 三层；✅ nil receiver 防护；✅ sync.Once 探测；无 panic |
| Lens-2 架构分层 | ✅ layout 是 Layer 0 扩展（仅依赖 core）未越界；✅ 8 层 Elm 不破 | ✅ doctmpl 通过 Renderer 接口解耦，chromedp 隔离在实现层；✅ RenderStyle 投影避免循环依赖 |
| Lens-3 安全红线 | N/A（纯渲染） | 🔴 **HTML 注入面**（WithUnsafe + 不转义 vars）；⚠️ no-sandbox；✅ 端口=0+本机；✅ 非敏感路径清单 |
| Lens-4 测试门禁 | ✅ 模式①②有测试；❌ **模式③ HasSuffix 无测试**；✅ Benchmark 覆盖流式 | ✅ 7 测试（Format/nil×2/端到端/探测/超时）；✅ Skipf 优雅；❌ 无注入/恶意 HTML 测试 |
| Lens-5 核心理念 | ✅ chat_history.go **622 行**（低于 750-899 关注）；✅ 复杂度已分散到 render*.go；无 TODO | ✅ CHANGELOG 完整；⚠️ chromedp 升 direct 依赖；✅ 优雅降级符合"去繁就简" |

## 3. 发现清单

| ID | 风险等级 | 类别 | 证据(文件:行) | 规范条款 | 建议 |
|----|---------|------|--------------|---------|------|
| **F-R13-1** | **H** | Lens-3 HTML 注入 | `tools.go:113-121` Variables 直接 json.Unmarshal 自工具 args（LLM 生成）；`renderer_html.go:36-38` `gmhtml.WithUnsafe()` 原始 HTML 透传不转义；`renderer_pdf_chrome.go:68-72` page.SetDocumentContent 注入 about:blank；全链路无 html.EscapeString（仅 Title/Author 在 head 转义，body vars 不转义） | 安全红线防御纵深 | 对 vars 做 HTML 实体转义后再插值；或评估能否移除 goldmark WithUnsafe()；补恶意 HTML 输入测试 |
| F-R13-2 | M | Lens-3 一致性 | `renderer_pdf_chrome.go:111-113` 字符串式 `chromedp.Flag("no-sandbox",true)` vs `tools/browser_supervisor.go:164-167` 类型化 `chromedp.NoSandbox` | 一致性 | 统一改用 chromedp.NoSandbox/chromedp.DisableGPU；no-sandbox 行加注释说明风险接受 |
| F-R12-1 | M | Lens-4 测试覆盖 | `chat_history.go:516-520` HasSuffix 分支实现存在；`chat_history_test.go` grep HasSuffix/suffix/ReEmit **零命中**；AI_CHANGELOG:92-95 声称三模式均已测试与实际不符 | 测试完整性 | 补 TestChatHistoryAppendDeltaSuppressesReEmittedSuffix（current="Hello, world", delta="world" 断言被抑制） |
| F-R12-2 | L | Lens-1 并发契约 | `tui_render.go:155,192,199,205-208` lastCursor 读写不持 t.mu；安全性来自 RequestRender→tickCh→单渲染 goroutine 串行化 | 并发契约显式化 | lastCursor 字段加注释"仅在 renderFrame（单渲染 goroutine）中读写，无需持 mu" |
| F-R12-3 | L | Lens-5 正面观察 | chat_history.go 实测 622 行（末行 627）；渲染已拆分到 chat_history_render{,_message,_highlight}.go 等 | — | 复杂度聚集已有效缓解，无需动作 |

## 4. 已验证合规项

| 项 | 证据 |
|----|------|
| ✅ **UTF-8 截断安全（#P1 不复发）** | `width.go:243,249` 用 `utf8.DecodeRuneInString` + `i+=size`，rune 边界对齐 |
| ✅ 截断保留 ANSI 样式 | `width.go:235-239,258-260` 跟踪 openStyles 并补 `\x1b[0m` |
| ✅ 双层溢出安全网 | `tui_render.go:97-100`（终极）+ `flex.go:295-308`（Flex 层），均从顶部裁剪保底部输入区 |
| ✅ chromedp 子进程生命周期 | `renderer_pdf_chrome.go:51-63` 三层 defer cancel + context.WithTimeout |
| ✅ 无端口冲突/未授权访问 | 依赖 DefaultExecAllocatorOptions 默认 port=0/127.0.0.1 |
| ✅ 优雅降级架构 | PDFAutoRenderer sync.Once 探测 + gopdf 回退；测试 t.Skipf |
| ✅ Renderer 接口契约不破 | renderer_pdf_chrome.go:35,141 实现 Renderer 接口 |
| ✅ 无循环依赖 | RenderStyle 投影（format.go:81）让 doctmpl 不依赖 domains |
| ✅ "变更即记录"满足 | AI_CHANGELOG R12 三条 + R13 记录完整 |
| ✅ 无 TODO/FIXME 残留 | doctmpl 包 grep 零命中 |
| ✅ 不在敏感路径清单 | renderer_pdf_chrome.go 不在 check-sensitive-paths.sh |
| ✅ nil receiver 防护 | renderer_pdf_chrome.go:39,146 双重 nil 检查 |

## 5. 建议下一步

| 优先级 | 动作 | 对应发现 |
|--------|------|----------|
| **P2** | 对 render_doc_template 的 vars 做 HTML 实体转义，或评估能否移除 goldmark WithUnsafe()；补恶意 HTML 输入测试 | F-R13-1 |
| **P3** | 补 TestChatHistoryAppendDeltaSuppressesReEmittedSuffix 测试，闭合模式③回归盲点 | F-R12-1 |
| **P4** | Chrome 标志位改用类型化常量（chromedp.NoSandbox 等），no-sandbox 行加风险注释 | F-R13-2 |
| **P4** | lastCursor 字段加并发契约注释 | F-R12-2 |

> **整体评价**：R12 渲染质量高——UTF-8 安全、双层防御、复杂度受控（chat_history.go 已降至 622 行），仅测试覆盖有一处小缺口。R13 架构清晰（接口隔离、优雅降级、生命周期完备），但 **HTML 注入面是真实的安全纵深缺口**，建议优先处理 F-R13-1。两块代码均符合"克制、中庸"理念，CHANGELOG 记录规范，可合入（建议 F-R13-1 后续 PR 跟进）。
