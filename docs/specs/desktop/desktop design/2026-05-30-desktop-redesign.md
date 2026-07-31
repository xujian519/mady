# BCIP 桌面端壳层重设计（摘要）

> [!WARNING]
> **DEPRECATED（归档）**：本文档为 BCIP 时代（2026-05-30）历史设计，
> 基于已废弃的 **Tauri 方案**。当前桌面端实现以
> `docs/specs/desktop/` 根目录下的 01-proposal/02-spec/03-design/04-tasks/05-pilotdeck-alignment
> 为准（Wails v2 + React）。仅保留作为设计历史参考。


**日期**: 2026-05-30
**状态**: 架构摘要（实现细节见配套文档）
**读者**: 产品、设计、桌面端开发

---

## 1. 一句话目标

保留 **专利三栏工作区**（项目树、文书预览、阶段/Todo），把 **唯一 Agent 会话** 放在右侧 Codex 对齐面板；中心区域不再承载第二套聊天 UI。

---

## 2. 信息架构

```
主壳（常驻）
├── TitleBar + StatusBar
├── LeftSidebar（项目 | 文件）
├── CenterWorkspace（阶段 | 预览/编辑 | Todo | 终端 overlay）
└── AgentPanel（线程 + 时间线 + Composer）

设置（/settings 或全屏 overlay）
└── 与 config.toml 一一对应（见 Codex parity 策略）

首启 / 故障
└── 检测 CLI → 附着 app-server → 进入主界面
```

---

## 3. 与旧版 `apps/desktop` 的差异

| 项 | 旧方案 | 本 redesign |
|----|--------|-------------|
| 中心对话 | `ChatView` 或 mock 工作流页 | **删除**；仅文件/阶段/Todo |
| 右侧对话 | 简化 `RightPanel` + WS | **Codex parity** `components/agent/*` |
| 配置 | 部分本地 mock | **app-server v2 RPC** |
| 传输 | `ws://127.0.0.1:9002` 默认 | **unix socket / stdio proxy**（见策略文档） |

---

## 4. 配套文档（阅读顺序）

1. [`2026-05-30-desktop-codex-parity-strategy.md`](2026-05-30-desktop-codex-parity-strategy.md) — 传输、单例 app-server、MCP/技能单一配置源
2. [`2026-05-30-desktop-design-spec.md`](2026-05-30-desktop-design-spec.md) — Figma/组件树/走查表
3. [`../codex-desktop-pixel-perfect-design-spec.md`](../codex-desktop-pixel-perfect-design-spec.md) — 像素级 Token 与双平台
4. [`2026-05-30-desktop-implementation-plan.md`](2026-05-30-desktop-implementation-plan.md) — **工程落地计划（主文档）**

`IMPLEMENTATION_PHASES.md` 仅作 Tauri/文件树/预览的历史任务参考；**协议与 Agent 以策略 + 落地计划为准**。

---

## 5. 验收要点（壳层）

- [ ] 中心无用户/助手气泡
- [ ] 右栏可完成 thread/turn、审批、MCP 状态展示
- [ ] 打开项目后 `thread/start.cwd` 指向项目根
- [ ] 阶段指示器 / Todo 与案件上下文一致（P2 可与 item 事件联动）

---

**文档结束**
