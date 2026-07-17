# P3 远期特性决策记录

> 日期: 2026-07-18 | 来源: docs/decisions/reasonix-analysis.md

## 决策摘要

reasonix-analysis.md 列出了 3 项 P3 长期引入特性。经过代码审查和需求分析，决策如下：

| 特性 | 决策 | 理由 |
|------|------|------|
| IM Bot（飞书/微信/钉钉） | **暂缓** | 专利/法律用户的即时通讯需求未验证；TUI + Web Server + ACP 已覆盖主要交互方式 |
| Desktop App（Wails） | **暂不做** | TUI 提供终端体验，serve 模式提供 Web 访问；桌面应用带来额外的平台维护负担 |
| Cache-First Architecture | **部分实现** | agentcore/cache/ 包已创建；tiered engine 的 prefix cache 优先策略已上线；进一步优化留待实际使用数据驱动 |

## IM Bot 详细分析

**触发条件:**
- 需要至少 3 个活跃用户明确表达 IM 集成需求
- 飞书优先（专利代理事务所常用），微信/企微跟进

**如果启动:**
- 优先实现飞书 Bot API 接入
- 实现为独立的 `cmd/mady-bot/` 入口
- 复用现有 Router → Domain Agent 架构
- 敏感操作（写文件/外部发送）需人工审批

## Desktop App 详细分析

**不做的理由:**
- TUI（mady tui）已提供全功能终端体验
- ACP 模式支持 Zed/VS Code 集成
- serve 模式提供 Web SSE 访问
- Wails 桌面应用增加 Go + React 双栈维护负担

## 相关文件

- docs/decisions/reasonix-analysis.md — 原始分析，P3 条目
- agentcore/cache/ — Cache-First Architecture 包
