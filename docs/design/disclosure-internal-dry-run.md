# Disclosure Internal Dry-Run

本文档用于 P3 前的内部彩排，目标是用最小成本验证 disclosure 主流程在本地可复跑、可复核、可留痕。

## 目标

- 验证 `analyze -> awaiting_review -> reviewed` 主路径可用
- 验证审批留痕链路已接通
- 验证 Markdown 导出路径仍可用
- 为后续真人试用提供统一操作口径

## 前置条件

- 已在仓库根目录执行过以下命令且通过：

```bash
make build
make test-race
make lint
```

- 已具备可启动 `mady serve` 的基础环境变量
- 本地 `~/.mady` 已存在知识库数据时效果最佳；没有也可做流程演练

## 推荐入口

优先跑自动化 gate：

```bash
make test-dry-run-gate
```

该命令会执行两部分：

- `make test-disclosure-smoke`
  - 验证最小 happy path：`analyze -> review -> export`
- `make test-approval-audit`
  - 验证 TUI / Server / ACP 三条人工决策留痕入口的记录语义一致

如果这一步失败，先不要做人工彩排，先修自动化回归。

## 手工彩排

### 1. 启动服务

```bash
PROVIDER=generic API_KEY=dummy BASE_URL=http://127.0.0.1:9 ./build/mady serve
```

说明：

- 仅用于验证启动与路由装配时，可使用占位 provider
- 如需真实分析质量，请替换成可用模型配置

### 2. 提交 disclosure 分析任务

```bash
curl -sS -X POST http://localhost:8080/v1/disclosure/analyze \
  -H 'Content-Type: application/json' \
  -d '{
    "text": "发明名称：一种低功耗运动检测传感器\n\n背景技术\n现有运动检测方案功耗高，影响可穿戴设备续航。\n\n发明内容\n本发明通过自适应采样率算法和低功耗休眠模式降低待机功耗。"
  }'
```

预期：

- 返回 `task_id`
- `status` 为 `pending`

### 3. 轮询任务状态

```bash
curl -sS http://localhost:8080/v1/disclosure/analyze/<task_id>
```

预期：

- 最终进入 `awaiting_review`
- `result.report_text` 非空
- `review_decision` 为空

### 4. 提交人工复核结论

采纳：

```bash
curl -sS -X POST http://localhost:8080/v1/disclosure/analyze/<task_id>/review \
  -H 'Content-Type: application/json' \
  -d '{
    "decision": "adopted",
    "feedback": "结构完整，可进入下一步处理",
    "case_id": "dry-run-case-1"
  }'
```

修改后采纳：

```bash
curl -sS -X POST http://localhost:8080/v1/disclosure/analyze/<task_id>/review \
  -H 'Content-Type: application/json' \
  -d '{
    "decision": "modified",
    "modified_output": "请补充一轮从权方案后再进入下一步。",
    "feedback": "核心方向可用，但建议补充从权",
    "case_id": "dry-run-case-1"
  }'
```

拒绝：

```bash
curl -sS -X POST http://localhost:8080/v1/disclosure/analyze/<task_id>/review \
  -H 'Content-Type: application/json' \
  -d '{
    "decision": "rejected",
    "feedback": "证据不足，需要补充现有技术比对",
    "case_id": "dry-run-case-1"
  }'
```

预期：

- 返回 `status=reviewed`
- 再次查询状态时 `review_decision` 已写入
- `result.reviewed_by_human` 为 `true`

### 5. 核对留痕

如果本地启用了审批存储，检查 `approvals.db` 中是否出现对应记录：

- `session_id = <task_id>`
- `case_id = dry-run-case-1`
- `trigger_keyword = disclosure_review`
- `decision` 与提交值一致

对于 TUI / ACP 留痕一致性，不建议手工点查；统一以：

```bash
make test-approval-audit
```

为准。

### 6. 核对导出

当前 HTTP 接口不直接暴露 export 端点，因此导出路径统一以自动化 smoke test 为准：

```bash
make test-disclosure-smoke
```

该测试已覆盖：

- report 生成
- review 后状态推进
- Markdown 导出成功
- 已复核报告不再带未复核警告

## 完成标准

- `make test-dry-run-gate` 通过
- 手工 `analyze -> review` 走通一次
- `awaiting_review` 和 `reviewed` 两个关键状态符合预期
- 对应审批记录可回溯
- 启动日志中没有阻断主流程的错误

## 常见现象

- `mcp: discovery timed out after 1.5s`
  - 属于启动期最佳努力降级，不阻断主流程
- `eval store ... 不可用`
  - 当前只影响评估数据持久化，不影响主服务和 disclosure 主流程
- 缺少 `pandoc`
  - 只影响 DOCX 导出，不影响 Markdown 导出和主流程验证

## 建议节奏

- 每次启动体验或 disclosure 主路径有改动后，先跑一次：

```bash
make test-dry-run-gate
```

- 需要给真人试用前，再补一次手工彩排
