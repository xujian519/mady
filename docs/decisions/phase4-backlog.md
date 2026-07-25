# Phase 4 审阅统一 Backlog

> 由 `Mady 全面审阅计划 v1.0` Phase 5 收敛生成。
> 时间：2026-07-25
> 覆盖范围：Phase 2（R6-R9 业务核心）、Phase 3（R10-R15 新增代码/规范）、Phase 4（R16-R17 数据隐私）的全部可执行发现。
> Human Owner：[NEEDS CLARIFICATION]

## 评分规则

| 字段 | 说明 |
|------|------|
| **Risk（风险等级）** | H=3, M=2, L=1, C=4（Critical） |
| **Cost（工时，人日）** | 基于代码改动 + 测试 + 文档同步的粗估 |
| **Score（风险×成本）** | Risk × Cost，值越大代表该事项的“风险-成本负担”越重 |
| **Priority** | 综合 `Risk / Cost`（性价比）与业务紧迫度给出的建议执行顺序 |

> **执行策略**：优先处理 `Risk/Cost` 高且 `Risk=H/C` 的事项；低分大项（如统一 redaction 层）需单独立项而非一次性完成。

## Backlog

| ID | 来源 | 标题 | 风险 | 成本 | Score | Priority | 建议负责模块 | 关键文件 |
|----|------|------|------|------|-------|----------|------------|----------|
| **1** ✅ | R17-02 | memory 查询强制按 ProjectID + SessionID 过滤，防止跨项目记忆泄漏 | H | 0.5 | 1.5 | P0 | memory | `memory/extension.go`, `memory/tools.go`, `memory/dedup.go`, `memory/preference.go`, `memory/types.go`, `memory/integration_test.go` |
| **2** ✅ | R17-11 | tasklist 按 workspace 分目录，废弃全局顺序数字 ID | H | 0.5 | 1.5 | P0 | agentcore/tasklist | `cmd/mady/framework.go`, `cmd/mady/framework_test.go` |
| **3** | R8-L2-1 | evidence 模块 3400 行死代码：**暂缓删除，等待其他窗口集成落地后再评估**；当前仅做引用审计与兼容性标记 | C | 0.5 | 2.0 | P1 | domains/evidence | `domains/evidence/` |
| **4** | R8-L3-1 | 侵权证明标准三重矛盾（clear_and_convincing / 高度盖然性）统一 | H | 0.5 | 1.5 | P1 | domains/evidence | `domains/evidence/*.go` |
| **5** | R9-F1 | psychological calming confidence 公式反向修复 | H | 0.5 | 1.5 | P1 | psychological | `psychological/engine.go:229-238` |
| **6** | R9-F2 | CBT 死开关：实现 SkipDistortionDetection 或移除该配置项 | H | 1.0 | 3.0 | P1 | psychological | `psychological/engine.go`, `psychological/config.go` |
| **7** | R6-F01 | inventiveness IsInventive 非对称 override 与 AND 公式一致化 | H | 1.0 | 3.0 | P1 | domains/inventiveness | `domains/inventiveness/nodes.go:786-797` |
| **8** | R7-F01 | enablement 9 个法律 case fixtures 的 expected 字段断言补齐 | H | 1.0 | 3.0 | P1 | domains/enablement | `domains/enablement/*_test.go` |
| **9** | R7-F02 | 26.3 提示词移除 In re Wands 美国案例，改用中国法域素材 | H | 0.5 | 1.5 | P1 | domains/enablement | `domains/enablement/prompt*.go` |
| **10** ✅ | R13-1 | PDF/HTML 渲染对模板变量做 HTML 实体转义，阻断 LLM 控制变量的注入链 | H | 1.0 | 3.0 | P1 | domains/doctmpl | `domains/doctmpl/store.go`, `domains/doctmpl/store_test.go` |
| **11** ✅ | R10-F1 | OrchestrationExecutor 增加递归深度限制，避免重复 handoff #P1 栈溢出 | H | 1.0 | 3.0 | P1 | agentcore | `agentcore/orchestration_executor.go` |
| **12** | R16-14 | Agent 主循环 outbound 前统一 PII redaction（message/tool 内容） | H | 3.0 | 9.0 | P2 | agentcore/provider | `agentcore/agent_provider.go`, `provider/chatcompat/chat.go` |
| **13** ✅ | R16-03 | Session Summarizer 接入 `SensitiveDataFilter`，摘要前过滤凭据 | H | 0.3 | 0.9 | P0 | memory | `memory/session_summarizer.go`, `memory/extractor_llm.go`, `memory/session_summarizer_test.go` |
| **14** | R16-06/11 | Embedding 链路（query/文档分片/记忆内容）统一 redact | H | 2.0 | 6.0 | P2 | retrieval/knowledge/memory | `retrieval/embedding.go`, `knowledge/sqlite/writable.go`, `memory/sqlite_store.go` |
| **15** | R16-15 | Context Compaction 压缩前对消息内容 redact | H | 1.0 | 3.0 | P1 | agentcore | `agentcore/compaction.go:462` |
| **16** | R8-L2-2 | evidence rule DSL 解析后从未执行：修复解释器接入或删除 | H | 1.5 | 4.5 | P2 | domains/evidence | `domains/evidence/engine*.go` |
| **17** | R9-F4 | psychological hook.go 整文件死代码：删除或补生命周期接入 | H | 0.5 | 1.5 | P1 | psychological | `psychological/hook.go` |
| **18** | R6-F02 | inventiveness nodes.go 1020 行拆分/重构 | H | 2.0 | 6.0 | P3 | domains/inventiveness | `domains/inventiveness/nodes.go` |
| **19** | R7-F03 | enablement 五场景 vs 六场景不一致澄清并统一 | H | 0.5 | 1.5 | P1 | domains/enablement | `domains/enablement/` |
| **20** | R8-L1-1 | evidence engine 重复计数修复 | M | 0.5 | 1.0 | P2 | domains/evidence | `domains/evidence/engine.go:165-176` |
| **21** | R14-1 | MCP C7 信任门 TOCTOU 加固（trust→load 之间复用已校验字节） | M | 0.5 | 1.0 | P2 | mcp | `mcp/config_discovery.go` |
| **22** | R14-2 | `isOwnedByCurrentUser` Unix/Windows 单测补齐 | M | 0.5 | 1.0 | P2 | mcp | `mcp/config_discovery_owner_{unix,windows}.go` |
| **23** | R12-1 | chat_history delta 去重第三模式（HasSuffix）补测试 | M | 0.3 | 0.6 | P2 | tui | `tui/chat/chat_history_test.go` |
| **24** | R10-F2 | Orchestration "YAML compiler" 文档与实现一致化 | M | 1.0 | 2.0 | P3 | agentcore | `agentcore/orchestration_executor.go` |
| **25** ✅ | R10-F3 | Orchestration 递归/深度相关单测补齐 | M | 0.5 | 1.0 | P2 | agentcore | `agentcore/orchestration_executor_test.go` |
| **26** | R6-F03 | inventiveness tool.go 导入 disclosure 层违规修复 | M | 1.0 | 2.0 | P2 | domains/inventiveness | `domains/inventiveness/tool.go` |
| **27** | R6-F04 | inventiveness mockProvider 空响应 callCount 断言 | M | 0.3 | 0.6 | P2 | domains/inventiveness | `domains/inventiveness/*_test.go` |
| **28** | R7-F04 | enablement 禁用词"必然"替换为带置信度表述 | M | 0.3 | 0.6 | P2 | domains/enablement | `domains/enablement/prompt*.go` |
| **29** | R12-2 | tui_render.go `lastCursor` 并发契约注释显式化 | L | 0.1 | 0.1 | P3 | tui | `tui/tui_render.go` |
| **30** | R13-2 | PDF Chrome `--no-sandbox` 统一为类型化 chromedp.NoSandbox | L | 0.2 | 0.2 | P3 | domains/doctmpl | `domains/doctmpl/renderer_pdf_chrome.go` |
| **31** | R14-3 | MCP 信任门边界写入 SECURITY.md | L | 0.1 | 0.1 | P3 | mcp/docs | `SECURITY.md` |
| **32** | R15-1/2 | AI_CHANGELOG 格式修订：保留四段式 + 强制 HO/Risk/Spec 占位 | M | 0.5 | 1.0 | P2 | docs/process | `CONTRIBUTING.md`, `docs/decisions/AI_CHANGELOG.md` |
| **33** | R17-05 | serve 模式下 session 按 workspace 分区 | M | 0.5 | 1.0 | P2 | cmd/server | `cmd/mady/server.go` |
| **34** | R17-08 | filecheckpoint 根目录动态跟随当前 project | M | 0.5 | 1.0 | P2 | agentcore/cmd | `cmd/mady/framework.go`, `cmd/mady/tui_session_agent.go` |
| **35** | R17-07 | filecheckpoint symlink 路径解析加固 | L | 0.3 | 0.3 | P3 | agentcore | `agentcore/filecheckpoint/store.go` |

## 按优先级分桶

### P0 — 立即修复（高危 + 低成本，1-2 天内）
1. **R17-02** memory 跨项目泄漏（H/0.5d）
2. **R17-11** tasklist 全局目录（H/0.5d）
3. **R16-03** Session Summarizer 零脱敏（H/0.3d）

### P1 — 高价值修复（H 级，1-3 天）
4. R8-L2-1 evidence 死代码处置（C/0.5d）
5. R8-L3-1 证明标准矛盾（H/0.5d）
6. R9-F1 calming confidence 公式反向（H/0.5d）
7. R9-F2 CBT 死开关（H/1d）
8. R6-F01 IsInventive 非对称 override（H/1d）
9. R7-F01 enablement fixtures 断言（H/1d）
10. R7-F02 26.3 提示词美国案例（H/0.5d）
11. R7-F03 五/六场景不一致（H/0.5d）
12. R13-1 PDF HTML 注入（H/1d）
13. R10-F1 orchestration 递归深度（H/1d）
14. R16-15 compaction redaction（H/1d）
15. R9-F4 hook.go 死代码（H/0.5d）

### P2 — 中等价值/需要设计（M 级或较大改动）
16. R16-14 Agent 主循环统一 redaction（H/3d）
17. R16-06/11 Embedding 链路 redaction（H/2d）
18. R8-L2-2 evidence DSL 未执行（H/1.5d）
19. R20 重复计数（M/0.5d）
20. R14-1 MCP TOCTOU（M/0.5d）
21. R14-2 owner 单测（M/0.5d）
22. R12-1 HasSuffix 测试（M/0.3d）
23. R10-F2 YAML compiler 一致化（M/1d）
24. R10-F3 递归测试（M/0.5d）
25. R6-F03 层违规（M/1d）
26. R6-F04 mock 断言（M/0.3d）
27. R7-F04 禁用词（M/0.3d）
28. R15 格式修订（M/0.5d）
29. R17-05 serve session 分区（M/0.5d）
30. R17-08 filecheckpoint 动态 root（M/0.5d）

### P3 — 长期债务/低紧迫度
31. R6-F02 nodes.go 拆分（H/2d）
32. R12-2 lastCursor 注释（L/0.1d）
33. R13-2 no-sandbox 类型化（L/0.2d）
34. R14-3 MCP 边界文档（L/0.1d）
35. R17-07 symlink 解析（L/0.3d）

## 风险聚合

| 风险等级 | 数量 | 占比 |
|----------|------|------|
| Critical (C) | 1 | 2.9% |
| High (H) | 20 | 57.1% |
| Medium (M) | 12 | 34.3% |
| Low (L) | 4 | 11.4% |
| **合计** | **35** | 100% |

> 注：Critical/H 级占比 60%，其中 data privacy（R16/R17）贡献 7 个 H/C，业务核心（R6-R9）贡献 10 个 H/C，新增代码（R10-R13）贡献 3 个 H。Phase 2 的"死代码/未实现"问题占最大比重。

## 工时汇总

| 优先级 | 项数 | 估算总工时 |
|--------|------|-----------|
| P0 | 3 | 1.3 人日 |
| P1 | 12 | 8.5 人日 |
| P2 | 15 | 11.6 人日 |
| P3 | 5 | 2.6 人日 |
| **总计** | **35** | **24.0 人日** |

## 关联文档

- Phase 2 报告：`docs/review/2026-07-25-phase2-{inventiveness,enablement,evidence,psychological}.md`
- Phase 3 报告：`docs/review/2026-07-25-phase3-{orchestration-tasklist,tui-pdf,mcp,changelog}.md`
- Phase 4 报告：`docs/review/2026-07-25-phase4-privacy-isolation.md`
- Phase 5 总结：`docs/review/2026-07-25-phase5-summary.md`

---

> 本 backlog 为审阅结论，未修改代码。Human Owner 待指派后可将 P0/P1 项拆分为独立任务单。
