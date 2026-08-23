#!/usr/bin/env python3
"""覆盖率门禁：多模块加权语句覆盖率 vs codecov.yml 同源阈值（fail-closed）。

用法:
  python3 scripts/check-coverage.py <profile1.out> [profile2.out ...]

判定:
  - 阈值单源解析自 codecov.yml 的 coverage.status.project.default.target − threshold
    （解析失败 → exit 1，无内置默认值，防误配静默放行）
  - ignore 列表同样解析自 codecov.yml（example/**、integration/** 等），
    与 CI 的 codecov 判定口径一致，避免本地红 CI 绿或反之
  - 加权合计 = 覆盖语句数 / 语句总数，跨全部传入 profile
  - 合计低于阈值 → exit 1（阈值是下限，round: down 按 codecov 语义取 floor）

定位：覆盖率是死代码探测器而非考核，故不进 make verify 链；由 make coverage-check 显式运行。
"""

import fnmatch
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent


def parse_codecov_yml() -> tuple[float, float, list[str]]:
    """解析 codecov.yml，返回 (target, threshold, ignore_patterns)。失败抛 ValueError。"""
    path = REPO_ROOT / "codecov.yml"
    text = path.read_text(encoding="utf-8")

    target = re.search(r"^\s*target:\s*([0-9.]+)%", text, re.MULTILINE)
    threshold = re.search(r"^\s*threshold:\s*([0-9.]+)%", text, re.MULTILINE)
    if not target or not threshold:
        raise ValueError(
            "codecov.yml 缺少 coverage.status.project.default.target/threshold，"
            "无法确定覆盖率阈值（fail-closed，拒绝内置默认值）"
        )

    # ignore 段：`ignore:` 行起，到下一个顶格键为止的 `  - "pattern"` 行
    patterns: list[str] = []
    ignore_match = re.search(r"^\s*ignore:\s*$", text, re.MULTILINE)
    if ignore_match:
        rest = text[ignore_match.end():]
        for line in rest.splitlines():
            if not line.startswith("  - "):
                break
            patterns.append(line[4:].strip().strip('"').strip("'"))

    return float(target.group(1)), float(threshold.group(1)), patterns


def iter_profile_entries(profile: Path):
    """逐行产出 (file_path, num_stmt, covered)。跳过 mode 行与空行。"""
    for line in profile.read_text(encoding="utf-8").splitlines():
        if not line or line.startswith("mode:"):
            continue
        # 格式: path/to/file.go:startLine.startCol,endLine.endCol numStmt count
        file_part, _, rest = line.partition(":")
        tokens = rest.split()
        if len(tokens) < 3:
            continue  # 非语句行（防御，正常 profile 不含）
        yield file_part, int(tokens[1]), int(tokens[2]) > 0


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print(f"用法: {Path(argv[0]).name} <profile1.out> [profile2.out ...]", file=sys.stderr)
        return 2

    try:
        target, threshold, ignore = parse_codecov_yml()
    except ValueError as e:
        print(f"❌ check-coverage: {e}", file=sys.stderr)
        return 1
    floor = target - threshold

    total_stmt = 0
    covered_stmt = 0
    print("模块覆盖率（codecov.yml 同源 ignore 口径）:")
    for profile_arg in argv[1:]:
        profile = Path(profile_arg)
        if not profile.exists():
            print(f"❌ check-coverage: profile 不存在: {profile}", file=sys.stderr)
            return 1
        mod_total = 0
        mod_covered = 0
        for file_path, num_stmt, is_covered in iter_profile_entries(profile):
            if any(fnmatch.fnmatch(file_path, p) for p in ignore):
                continue
            mod_total += num_stmt
            if is_covered:
                mod_covered += num_stmt
        total_stmt += mod_total
        covered_stmt += mod_covered
        pct = (mod_covered / mod_total * 100) if mod_total else 0.0
        print(f"  {profile.name:<14} {pct:5.2f}%  ({mod_covered}/{mod_total})")

    if total_stmt == 0:
        print("❌ check-coverage: 无有效语句（全部被 ignore 或 profile 为空）", file=sys.stderr)
        return 1

    overall = covered_stmt / total_stmt * 100
    print(f"  合计           {overall:5.2f}%  ({covered_stmt}/{total_stmt})")
    print(f"阈值: codecov.yml target {target:.0f}% − threshold {threshold:.0f}% = floor {floor:.0f}%")

    if overall < floor:
        print(
            f"❌ check-coverage: 合计 {overall:.2f}% < 阈值 {floor:.0f}%。"
            "阈值单源在 codecov.yml（勿在此脚本内置默认值）；若要调整请先补测试或改 codecov.yml。",
            file=sys.stderr,
        )
        return 1
    print(f"✓ check-coverage: {overall:.2f}% ≥ {floor:.0f}% 通过")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
