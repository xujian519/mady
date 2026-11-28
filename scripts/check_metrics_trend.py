#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""指标趋势门禁（防代码气味指标反弹）。

定位：`docs/code-health/metrics-trend.md` 的机器验证部分（AGENTS.md 元规则「三件套」）。
读 `docs/code-health/metrics-baseline.json`（基线单源），用 `scripts/scan_go_smells.py`
的同一套分析逐模块比对：
  - 当前 > 基线 → **失败**（新增气味即红，防注释被回退、导出注释被删、长函数膨胀）
  - 当前 < 基线 → 通过并提示可棘轮下压（`--update`）
  - 基线缺失/损坏/字段缺失 → **失败**（fail-closed，不静默放行）

净指标口径：`cli_fmt`（CLI 输出层 fmt，规范豁免）与 `files`（可增长）不设阈值。

用法:
  scripts/check_metrics_trend.py             # 比对（门禁模式，make metrics-trend 使用）
  scripts/check_metrics_trend.py --update    # 用当前实测值重写基线（棘轮下沉）
  scripts/check_metrics_trend.py --root DIR  # 指向其他仓库根（scripts/gatechecks 负控制用）
"""

import argparse
import datetime
import json
import os
import subprocess
import sys

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))

# 受门槛约束的指标（scan_go_smells.analyze_file 的键名）。
GATED_METRICS = (
    "bare_fmt",            # 库代码裸 fmt（CLI 输出层走 cli_fmt，不计）
    "ignored_err",         # 被忽略的 error
    "silent_return",       # 静默吞错无意图注释
    "uncommented_exported",  # 未注释包级导出
    "long_funcs",          # >120 行单函数（只防增长，不强制下降）
    "fixme",               # TODO/FIXME/HACK 标注
)

# 模块 → scan_go_smells 的扫描目录。root 用其默认范围（31 个目录）。
MODULES = ("root", "tui", "desktop")


def _repo_root(explicit: str | None) -> str:
    """仓库根：显式 --root 优先，否则取当前 cwd 所在 git 仓库（与 check-tone-words 一致，
    便于门禁自测在临时仓库内运行）；取不到则 fail-closed。"""
    if explicit:
        return os.path.abspath(explicit)
    try:
        out = subprocess.run(["git", "rev-parse", "--show-toplevel"],
                             capture_output=True, text=True, check=True).stdout.strip()
    except (OSError, subprocess.CalledProcessError) as exc:
        print(f"✗ 无法确定仓库根（git rev-parse 失败）: {exc}（fail-closed）", file=sys.stderr)
        sys.exit(1)
    return out


def _load_scan(root: str):
    """按需加载扫描器；扫描根通过 MADY_SCAN_ROOT 注入（与脚本自身的覆盖入口一致）。"""
    os.environ["MADY_SCAN_ROOT"] = root
    sys.path.insert(0, SCRIPT_DIR)
    import scan_go_smells  # noqa: E402  （延迟导入，确保环境变量先生效）

    return scan_go_smells


def _scan_module(scan, module: str) -> dict:
    dirs = list(scan.DEFAULT_DIRS) if module == "root" else [module]
    return scan.scan_dirs(dirs)["totals"]


def _load_baseline(path: str) -> dict:
    try:
        with open(path, encoding="utf-8") as fh:
            data = json.load(fh)
    except OSError as exc:
        print(f"✗ 无法读取基线 {path}: {exc}（fail-closed）", file=sys.stderr)
        sys.exit(1)
    except json.JSONDecodeError as exc:
        print(f"✗ 基线 JSON 解析失败 {path}: {exc}（fail-closed）", file=sys.stderr)
        sys.exit(1)
    modules = data.get("modules")
    if not isinstance(modules, dict):
        print(f"✗ 基线缺少 modules 字段 {path}（fail-closed）", file=sys.stderr)
        sys.exit(1)
    for m in MODULES:
        if m not in modules:
            print(f"✗ 基线缺少模块 {m}（fail-closed）", file=sys.stderr)
            sys.exit(1)
        for k in GATED_METRICS:
            if not isinstance(modules[m].get(k), int):
                print(f"✗ 基线 {m}.{k} 缺失或非整数（fail-closed）", file=sys.stderr)
                sys.exit(1)
    return data


def _render(rows: list[tuple[str, str, int, int]]) -> str:
    out = [
        "指标趋势校验（基线 = docs/code-health/metrics-baseline.json）",
        "=" * 52,
        f"  {'模块':<8}{'指标':<22}{'基线':>6}{'当前':>6}  状态",
    ]
    for module, metric, base, cur in rows:
        if cur > base:
            status = "✗ 反弹"
        elif cur < base:
            status = "↓ 可收紧"
        else:
            status = "✓"
        out.append(f"  {module:<8}{metric:<22}{base:>6}{cur:>6}  {status}")
    return "\n".join(out)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--root", help="仓库根（默认脚本所在仓库；门禁自测用）")
    ap.add_argument("--baseline", help="基线文件（默认 <root>/docs/code-health/metrics-baseline.json）")
    ap.add_argument("--update", action="store_true", help="用当前实测值重写基线（棘轮下沉）")
    ap.add_argument("--allow-increase", action="store_true",
                    help="配合 --update 允许上调基线（需在提交说明写明理由）")
    args = ap.parse_args()

    scan = _load_scan(_repo_root(args.root))
    root = scan.REPO_ROOT
    baseline_path = args.baseline or os.path.join(
        root, "docs", "code-health", "metrics-baseline.json")

    current = {m: _scan_module(scan, m) for m in MODULES}

    if args.update:
        baseline = _load_baseline(baseline_path)
        rises = [
            (m, k, baseline["modules"][m][k], current[m][k])
            for m in MODULES for k in GATED_METRICS
            if current[m][k] > baseline["modules"][m][k]
        ]
        if rises and not args.allow_increase:
            print("✗ 检测到指标上调，拒绝自动更新基线（确需放宽请加 --allow-increase 并写明理由）：",
                  file=sys.stderr)
            print(_render(rises), file=sys.stderr)
            return 1
        baseline["updated"] = datetime.date.today().isoformat()
        baseline["modules"] = {m: {k: current[m][k] for k in GATED_METRICS} for m in MODULES}
        with open(baseline_path, "w", encoding="utf-8") as fh:
            json.dump(baseline, fh, ensure_ascii=False, indent=2)
            fh.write("\n")
        print(f"✅ 基线已更新: {baseline_path}")
        return 0

    baseline = _load_baseline(baseline_path)
    rows: list[tuple[str, str, int, int]] = []
    regressions: list[tuple[str, str, int, int]] = []
    for m in MODULES:
        for k in GATED_METRICS:
            base = baseline["modules"][m][k]
            cur = current[m][k]
            rows.append((m, k, base, cur))
            if cur > base:
                regressions.append((m, k, base, cur))

    print(_render(rows))
    if regressions:
        print("\n✗ 指标反弹（高于基线）——修复后用 --update 棘轮下压基线：", file=sys.stderr)
        for m, k, base, cur in regressions:
            print(f"  {m}.{k}: 基线 {base} → 当前 {cur}", file=sys.stderr)
        return 1
    print(f"\n✓ 指标趋势校验通过（基线日期 {baseline.get('updated', 'unknown')}）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
