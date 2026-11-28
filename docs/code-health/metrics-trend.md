# 代码气味指标趋势看板

> 定位：把 `docs/code-review-report.md` §二 的终审指标固化为**不可回退的机器门禁**。
> 规范家 = 本文；机器验证 = `scripts/check_metrics_trend.py` + `scripts/gatechecks/`
> 红绿自测（`make verify` 与 CI 的 `metrics-trend` job 执行）；理由见 §3。
>
> 基线单源：`docs/code-health/metrics-baseline.json`（只由显式提交更新，CI 永不写基线）。

## 1. 受控指标

| 指标 | 含义 | 阈值语义 |
| --- | --- | --- |
| `bare_fmt` | 库代码裸 `fmt.Print*`（CLI 输出层走 `cli_fmt`，见 `docs/cli-output-standards.md`） | 不得高于基线 |
| `ignored_err` | 被忽略的 error（`_ = <expr>` 等，GO-STANDARDS §0.1#1） | 不得高于基线 |
| `silent_return` | 静默吞错且无意图注释 | 不得高于基线 |
| `uncommented_exported` | 未注释的包级导出符号 | 不得高于基线 |
| `long_funcs` | >120 行单函数（只防增长，不强制下降；清单见 `docs/code-health/long-function-inventory.md`） | 不得高于基线 |
| `fixme` | `TODO` / `FIXME` / `HACK` 标准标注 | 不得高于基线 |

**不设阈值**：`cli_fmt`（CLI 输出层规范豁免，数量随命令增减是正常现象）、`files`
（随功能增长）。二者仍在扫描报告中可见，只是不参与比对。

## 2. 使用方式

```bash
make metrics-trend                                    # 门禁模式（verify / CI 使用）
python3 scripts/check_metrics_trend.py                 # 同上，可直接看逐项对比表
python3 scripts/check_metrics_trend.py --update        # 指标下降后棘轮下压基线
python3 scripts/check_metrics_trend.py --update --allow-increase   # 确需放宽（写明理由）
```

行为约定：

- **高于基线 → exit 1**，逐项列出「基线 → 当前」，并停在这一步等修复；
- **低于基线 → exit 0**，提示可 `--update` 收紧（不自动改写，避免把偶发波动写进基线）；
- **基线缺失 / JSON 损坏 / 字段缺失 → exit 1**（fail-closed，不把「没跑」当「通过」）；
- `--update` 检测到上调时默认拒绝，必须显式加 `--allow-increase`，并在提交说明里写明
  理由（对应 AGENTS.md「例外显式」元规则）。

## 3. 为什么这么设计

1. **指标是审阅成果，不守就会回退**：M37–M45 把吞错补注到 0、导出注释补到 0、
   忽略 err 清零——这些成果全靠注释与约定支撑，一次重写就可能悄悄丢失。把「当前值」
   写成基线，任何新增都变成红，回归成本从「下次审阅才发现」降到「本次 CI 就拦」。
2. **只收紧不放宽（棘轮）**：基线只能通过显式提交下压，且上调需要理由；避免「指标涨了
   就改基线」的静默漂移。
3. **口径与扫描器同源**：门禁直接复用 `scripts/scan_go_smells.py` 的分析函数
   （`MADY_SCAN_ROOT` 注入根目录），不另写一套正则——避免「门禁算一套、报告算另一套」。
4. **门禁自身有负控制**：`scripts/gatechecks/gate_test.go` 的
   `TestMetricsTrendGate_Red_Regression`（高于基线必拦）、`_Green_AtBaseline`（持平放行）、
   `_Red_MissingBaseline`（基线缺失 fail-closed）证明它真的会红，而不是永远绿。
5. **不进 pre-commit**：指标是全仓快照，pre-commit 只应拦本次改动的增量问题
   （lint 已按 `--new-from-rev` 增量执行）；全仓指标放在 `make verify` 与 CI，
   与「本地窄 / CI 全」的元规则一致。

## 4. 当前基线快照（2026-11-28）

| 模块 | bare_fmt | ignored_err | silent_return | uncommented_exported | long_funcs | fixme |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| root | 0 | 0 | 0 | 0 | 35 | 3 |
| `tui/` | 0 | 0 | 0 | 0 | 10 | 0 |
| `desktop/` | 0 | 0 | 0 | 0 | 0 | 0 |

快照以 `metrics-baseline.json` 为准；本表仅供人读，漂移时以 JSON + 门禁输出为准。

## 5. 相关文档

- `docs/code-health/metrics-baseline.json` — 基线单源
- `scripts/check_metrics_trend.py` — 门禁实现（比对 / `--update` / fail-closed）
- `scripts/scan_go_smells.py` — 指标口径定义（`--json` 供脚本消费）
- `docs/code-review-report.md` — 指标基线与终审结论
- `docs/code-health/long-function-inventory.md` — 长函数清单与拆解分级
