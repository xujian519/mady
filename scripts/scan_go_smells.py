#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Go 代码气味扫描（每日审阅计数 / 基线测量）。

定位：Mady 每日代码审阅日卡（docs/code-review-plan.md）的机器计数工具。
兑现 AGENTS.md「规范元规则·事实清单可验证」——基线（§六）与每日卡进度表里的
指标计数必须可脚本复现，靠自觉维护的清单视为不存在。

只做**机械近似统计**（启发式），不替代人工审阅：
  - 裸 fmt.Print/Println/Printf/Fprint/Fprintln 调试残留（应走结构化 log/tracing/i18n，
    GO-STANDARDS §0.1#4；不含 Fprintf——带显式 writer 多为写文件/错误输出，全仓 683 处计入会淹没信号）
  - 被忽略的 error（`_ = <expr>`、RHS 显式含 err 字样的 `x, _ := f()`，GO-STANDARDS §0.1#1；
    无法静态判断丢弃值是否 error，宁可漏报不误报）
  - 静默吞错无意图注释（`if err != nil { return ... }` / `if err := f(); err != nil { ... }`
    且错误分支 return 前无注释——Go 无 catch，对应 TS 侧「无参 catch 缺注释」，
    见日卡审阅清单 #3；块扫描为花括号近似，嵌套块可能漏判）
  - TODO/FIXME/HACK/XXX 标注（独立注释行 + 行尾注释都计；逐条核实业务语义，见清单 #9）
  - 未注释导出符号（导出函数/类型/变量缺 `// SymbolName 中文描述。`，GO-STANDARDS §0.1#8；
    仅报告计数，供后续人工按清单 #7 核查）
  - 超长单函数（启发式：>120 行的函数体，GO-STANDARDS §0.2 过度工程化、日卡清单 #5；
    花括号深度含注释/字符串行，为近似计数）

用 Python 而非 shell/awk：macOS BSD awk 对多字节 UTF-8 字符串比较不可靠
（见 check-comment-seams.py 注记），且 Go 多文件跨目录统计在 Python 中更稳。

用法:
  scripts/scan_go_smells.py [DIRS...]       # 扫描指定目录（默认根模块 go 源文件范围）
  scripts/scan_go_smells.py --all-modules   # 含 tui/ desktop/ 子模块
  scripts/scan_go_smells.py --json          # 输出 JSON（供脚本消费）

退出码：总是 0（计数工具，非门禁）。--fail 时命中裸 fmt 即 exit 1。
"""

import argparse
import json
import os
import re
import subprocess
import sys

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(SCRIPT_DIR)

# 默认扫描范围：根模块（go.work 的 "."）。tui/ desktop/ 为独立 go.mod 子模块，
# 需要时显式传入或 --all-modules（跨模块统一计数会稀释单模块报告）。
DEFAULT_DIRS = (
    "agentcore", "a2a", "a2ui", "acp", "agui", "disclosure", "domains", "doomloop",
    "graph", "guardrails", "knowledge", "mcp", "memory", "psychological", "provider",
    "retrieval", "server", "session", "skill", "skills", "store", "tracing", "tools",
    "bootstrap", "evaluate", "intent", "cmd", "example", "fuzzy", "prompt", "pkg",
)
SUBMODULES = ("tui", "desktop")

# 排除目录（第三方样例、生成产物、前端源码不属于 Go 审阅范围）。
EXCLUDE_DIR_PARTS = ("vendor", "node_modules", ".git", "third_party")

# 启发式正则（跨行用 re.MULTILINE 处理）。
_BARE_FMT_RE = re.compile(r"\bfmt\.(Print|Println|Printf|Fprint|Fprintln)\(")
# 被忽略的 error：只匹配显式丢弃错误值的赋值（_ = expr 或多值返回中的 err 被 _ 接收）
_IGNORED_ERR_RE = re.compile(r"^\s*_ = [^\n]*(err|Err)|,\s*_\s*:?=.*\b(err|Err)\b")
_SILENT_RETURN_RE = re.compile(r"^\s*if (?:\w+\s*:=\s*[^;]+;\s*)?err != nil\s*\{\s*$")
_FIXME_RE = re.compile(r"\b(TODO|FIXME|HACK|XXX)\b")
_LONG_FUNC_OPEN_RE = re.compile(r"^func(?:\s*\([^)]*\))?\s+\w+\s*\(")


def _git_tracked_go_files(dirpath: str, include_tests: bool) -> list[str]:
    """返回 dirpath 下 git 跟踪的 .go 文件（相对仓库根）。

    include_tests=False（默认）排除 *_test.go：测试函数与接口方法实现是 Go 惯例
    （无需导出注释），计数测试文件会淹没真实信号（见 scan_go_smells.py docstring）。
    """
    try:
        out = subprocess.run(
            ["git", "ls-files", "--", os.path.join(dirpath, "*.go")],
            capture_output=True, text=True, check=True, cwd=REPO_ROOT,
        ).stdout
    except subprocess.CalledProcessError as exc:
        # 任何 cwd 下运行都相对仓库根解析 pathspec；失败要响亮（fail-closed），
        # 不静默返回 0 文件假基线（曾出现在子目录运行时 pathspec 相对 cwd 解析出错）。
        print(f"警告: git ls-files 失败（目录 {dirpath}）: {exc}", file=sys.stderr)
        return []
    files = []
    for l in out.splitlines():
        if not l or any(seg in EXCLUDE_DIR_PARTS for seg in l.split("/")):
            continue
        if not include_tests and l.endswith("_test.go"):
            continue
        files.append(l)
    return files


def _is_exported_func(line: str) -> bool:
    """是否为「包级导出函数」（func Name），方法（func (r T) ...）不算。

    导出注释要求针对包级 API（GO-STANDARDS §0.1#8）；接口实现方法靠接口文档，
    对方法逐个要求注释是噪音（见 doomloop 抽查：6 个 detector 的 ID/Record* 方法）。
    """
    if not line.startswith("func"):
        return False
    rest = line[4:].strip()
    if rest.startswith("("):
        return False  # 方法：排除
    return bool(re.match(r"[A-Z]\w*", rest))


def analyze_file(path: str) -> dict:
    """对单个 Go 源文件做启发式气味统计。"""
    stats = {
        "files": 1,
        "bare_fmt": 0,
        "ignored_err": 0,
        "silent_return": 0,
        "fixme": 0,
        "uncommented_exported": 0,
        "long_funcs": 0,
    }
    try:
        with open(path, encoding="utf-8") as fh:
            lines = fh.readlines()
    except (OSError, UnicodeDecodeError):
        return stats

    # main 包：fmt.Print* 是 CLI/示例的正常输出，不计入裸 fmt 调试残留。
    # package 声明可能不在文件头部（license 头/生成注释），扫全文件。
    is_main_pkg = any(l.strip() == "package main" for l in lines)

    brace_depth = 0
    func_start = None
    for i, raw in enumerate(lines):
        line = raw.rstrip("\n")

        # --- 全局统计（逐行）---
        stripped = line.lstrip()
        if not (stripped.startswith("//") or stripped.startswith("*")):
            if not is_main_pkg:
                stats["bare_fmt"] += len(_BARE_FMT_RE.findall(line))
            stats["ignored_err"] += len(_IGNORED_ERR_RE.findall(line))

        # TODO/FIXME/HACK/XXX 的核实对象是注释（清单 #9）：独立注释行与行尾注释都计，
        # 代码字符串中的 TODO 字样不算（如 tui 的 todo_panel 组件名、XXXX 年份）。
        comment = line.split("//", 1)
        if len(comment) == 2:
            stats["fixme"] += len(_FIXME_RE.findall(comment[1]))

        # 静默吞错：if err != nil { ... return 值 } 且错误分支无注释。
        # 只计「返回零值/降级」的吞错，排除 return err / return nil, err 的正确传播。
        if _SILENT_RETURN_RE.match(line):
            # 找同块内的 return（不精确：仅数到下一个 `}` 前是否出现 return 且中间无 //）
            j = i
            while j < len(lines) and "}" not in lines[j]:
                j += 1
            block = "".join(lines[i:j + 1])
            if re.search(r"return\b", block) and not re.search(r"//|/\*", block):
                # 错误分支「返回零值/降级」且丢弃 err —— 记为吞错；
                # 返回 err / nil, err / ..., err 属正确传播，不计。
                is_propagation = re.search(
                    r"return[^\n]*(,|\s)\s*err\b|return\s+err\b",
                    block,
                )
                if not is_propagation:
                    stats["silent_return"] += 1

        # 超长函数（启发式：函数体 >120 行）
        if _LONG_FUNC_OPEN_RE.match(line):
            func_start = i
            brace_depth = line.count("{") - line.count("}")
        elif func_start is not None:
            brace_depth += line.count("{") - line.count("}")
            if brace_depth <= 0:
                if i - func_start > 120:
                    stats["long_funcs"] += 1
                func_start = None

        # 未注释导出符号（仅包级导出函数/类型/变量；方法靠接口文档不逐方法要求）
        if (_is_exported_func(line) or re.match(r"^type\s+[A-Z]", line)
                or re.match(r"^var\s+[A-Z]", line)):
            # 向前找紧邻注释：上一条非空行必须以 // 或 /* 开头，且与声明间无空行
            has_comment = False
            k = i - 1
            while k >= 0 and not lines[k].strip():
                k -= 1
            if k >= 0 and lines[k].lstrip().startswith(("//", "/*", "*")):
                has_comment = True
            if not has_comment:
                stats["uncommented_exported"] += 1

    return stats


def scan_dirs(dirs: list[str], include_tests: bool = False) -> dict:
    totals = {k: 0 for k in ("files", "bare_fmt", "ignored_err", "silent_return",
                             "fixme", "uncommented_exported", "long_funcs")}
    per_dir: dict[str, dict] = {}
    for d in dirs:
        if not os.path.isdir(os.path.join(REPO_ROOT, d)):
            continue
        files = _git_tracked_go_files(d, include_tests=include_tests)
        acc = {k: 0 for k in totals}
        for f in files:
            s = analyze_file(os.path.join(REPO_ROOT, f))
            for k in totals:
                acc[k] += s[k]
                totals[k] += s[k]
        per_dir[d] = acc
    return {"totals": totals, "per_dir": per_dir}


def render_human(res: dict) -> str:
    lines = []
    t = res["totals"]
    lines.append("Mady Go 代码气味扫描")
    lines.append("=" * 40)
    lines.append(f"扫描目录数: {len(res['per_dir'])}    Go 源文件: {t['files']}")
    lines.append("")
    lines.append("  气味指标            命中数")
    lines.append("  " + "-" * 32)
    rows = [
        ("裸 fmt.Print/Println", t["bare_fmt"]),
        ("被忽略的 error", t["ignored_err"]),
        ("静默吞错无注释", t["silent_return"]),
        ("TODO/FIXME/HACK", t["fixme"]),
        ("未注释导出符号", t["uncommented_exported"]),
        (">120 行单函数", t["long_funcs"]),
    ]
    for name, n in rows:
        lines.append(f"  {name:<18} {n:>8}")
    lines.append("")
    lines.append("按目录明细:")
    for d, a in sorted(res["per_dir"].items(), key=lambda x: x[1]["files"], reverse=True):
        lines.append(
            f"  {d:<22} 文件{a['files']:>5}  裸fmt{a['bare_fmt']:>4}  "
            f"忽略err{a['ignored_err']:>4}  吞错{a['silent_return']:>4}  "
            f"TODO{a['fixme']:>4}  未注释{a['uncommented_exported']:>4}  长func{a['long_funcs']:>4}"
        )
    return "\n".join(lines)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("dirs", nargs="*", help="要扫描的目录（默认根模块范围）")
    ap.add_argument("--all-modules", action="store_true", help="含 tui/ desktop/ 子模块")
    ap.add_argument("--include-tests", action="store_true",
                    help="计入 *_test.go（默认排除，测试函数无需导出注释）")
    ap.add_argument("--json", action="store_true", help="输出 JSON")
    ap.add_argument("--fail", action="store_true", help="命中裸 fmt 即 exit 1（门禁模式）")
    args = ap.parse_args()

    dirs: list[str] = list(args.dirs) if args.dirs else list(DEFAULT_DIRS)
    if args.all_modules:
        dirs += list(SUBMODULES)

    res = scan_dirs(dirs, include_tests=args.include_tests)
    if args.json:
        print(json.dumps(res, ensure_ascii=False, indent=2))
    else:
        print(render_human(res))

    if args.fail and res["totals"]["bare_fmt"] > 0:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
