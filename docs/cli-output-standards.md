# Mady CLI 输出规范

> 定位：本文是 `cmd/`（CLI 入口与子命令）终端输出的**唯一权威定义处**。
> 兑现 AGENTS.md「规范元规则·三件套」：家 = 本文；机器验证 = `scripts/scan_go_smells.py`
> 的 `cli_fmt` 分类（`--fail` 只拦库代码裸 fmt）；理由 = 本文 §3。
>
> 与 `docs/GO-DEVELOPMENT-STANDARDS.md` 的关系：该文档约束**库代码**（日志走
> 结构化 `log`/`slog`/`tracing`/`pkg/i18n`）；本文约束**CLI 展示层**，两者互补，
> 不构成豁免库代码的理由。

---

## 1. 适用范围

| 属于 CLI 输出层（本文约束） | 不属于（按库代码规范） |
|---|---|
| `cmd/**` 下所有文件（`package main` 与 `package subcmd`） | `agentcore/`、`domains/`、`tools/`、`knowledge/` 等库代码 |
| `example/**`、`scripts/**` 下的 `package main` 示例与工具 | 库代码中被 CLI 调用的实现函数 |

判据只有一条：**代码是否直接面向终端用户展示结果**。库代码不得因被 CLI 调用而
升格为输出层；需要展示时由 CLI 层接收返回值后自行打印。

## 2. 输出通道约定

| 内容 | 通道 | 写法 |
|---|---|---|
| 命令正常结果（列表、统计、报告正文、`--help` 成功路径） | stdout | `fmt.Print` / `fmt.Println` / `fmt.Printf` |
| 错误、用法说明、进度、状态提示、产物保存路径 | stderr | `fmt.Fprintf(os.Stderr, ...)` / `fmt.Fprintln(os.Stderr, ...)` |
| 退出码 | — | 成功 `0`，失败非 `0`（由返回 `error` 决定，不在输出层自造码） |
| 运行期诊断日志 | stderr | `slog`（结构化字段，不用 `fmt` 拼接） |

**理由**：stdout 必须可被管道消费（如 `mady util list-prompts | grep`）。进度条、
状态提示、错误若混入 stdout，会污染下游解析；反之把结果写 stderr 会让
`mady ... > out.json` 拿到空文件。

## 3. 为什么 CLI 层豁免「裸 fmt」指标

`fmt.Print*` 在库代码中是调试残留信号（应走结构化日志），但在 CLI 层就是**输出
手段本身**——所有终端交互工具都如此。因此 `scripts/scan_go_smells.py` 把
`cmd/` 与 `package main` 的命中单独计入 `cli_fmt`（报告中展示为「CLI/示例输出层
fmt（豁免）」），不计入库代码的「裸 fmt」指标；`--fail` 门禁仍只对库代码生效。

改写为 `slog` / `pkg/i18n` 不等于提升质量——那只是把终端输出绕道日志系统，
反而丢失「结果可管道化」的语义。**豁免只针对通道，不针对内容**：文案仍受
`docs/tone-style-guide.md` 约束。

## 4. 当前基线（2026-11-28 实测）

`python3 scripts/scan_go_smells.py` 口径：

| 范围 | 处数 | 说明 |
|---|---|---|
| `cmd/mady/subcmd`（package subcmd） | 65 | eval 14 / trust_knowledge 17 / mcp_install 13 / ocr 8 / util 5 / evidence 4 / patent 3 / trust_mcp 1 |
| `cmd/mady/main.go` + `cmd/export-benchmark/main.go` | 31 | package main |
| `example/*/main.go` | 83 | 示例程序 |
| **合计 `cli_fmt`** | **179** | 库代码裸 fmt 为 **0** |

## 5. 新增豁免与变更流程

1. 新代码落在 `cmd/` 或 `example/`、`scripts/` 的 `package main` 内 → 自动归入
   `cli_fmt`，无需逐处声明。
2. 若确有库代码必须直接写终端（例如 framework 级交互），**不得**扩大
   `CLI_OUTPUT_DIRS` 覆盖范围；改为在调用点用行内
   `// scan-ignore: bare_fmt — <原因>` 显式豁免并写明理由，供审阅复核。
3. 修改本文的通道约定或判据时，必须同步 `scripts/scan_go_smells.py` 的
   `CLI_OUTPUT_DIRS` 与 docstring。两者一致性目前由 code review 保证（本仓未配置
   自动比对脚本），变更须在提交说明里写明同步点——遵循 GO-STANDARDS §0.4
   「标注什么就真能跑出什么」，不虚构机器检查。

## 6. 相关文档

- `docs/GO-DEVELOPMENT-STANDARDS.md` — 库代码错误处理与日志规范
- `docs/tone-style-guide.md` — 面向用户文案措辞与禁用词表
- `docs/code-review-plan.md` — 日卡审阅清单（裸 fmt 项口径）
