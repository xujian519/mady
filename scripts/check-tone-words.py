#!/usr/bin/env python3
"""tone-style 禁用词门禁（面向用户文案）。

扫描面向用户文案目录（guardrails/、server/、agui/、a2ui/、pkg/i18n/）中被 git
跟踪的 Go 源文件，检查禁用/慎用词（绝对、一定、百分百、保证、从不、无法处理、
暂不支持、存在侵权、构成冲突等），命中即失败。定位：面向用户文案的措辞门禁
（AGENTS.md 安全红线「护栏文案、报告结论措辞」），pre-commit 已接线。

词表自动解析自 docs/tone-style-guide.md「## 4. 禁用词表」表格——单源解析，
禁止在本脚本内手写词表（防词表与文档漂移，见 AGENTS.md「规范元规则」三件套）。

扫描范围限定 SCAN_DIRS（面向用户文案目录），跳过代码注释（// 之后），
排除测试文件（测试数据是被检测的输入而非产出文案，不受措辞约束），
EXEMPT 为文件级显式豁免——豁免即理由记录（AGENTS.md「例外显式」元规则）：
- guardrails/citation_table.go   — S1 静态主题表关键词（「不构成侵权」等是法律
  概念本身的收录项，非文案断言）
- guardrails/consistency.go      — 护栏一致性检测的检测词表自身定义（被检测词
  作为数据出现，豁免即检测目标）
- guardrails/fact_check.go       — 绝对化表述检测器自身的检测文案（「绝对化表述」
  是被检测现象名，命中词「绝对」属于该文件的定义范畴）

用法:
  scripts/check-tone-words.py           # 只报告命中统计（exit 0，排查用）
  scripts/check-tone-words.py --fail    # 命中即 exit 1（门禁模式，pre-commit/Makefile 使用）
"""

import os
import re
import subprocess
import sys

# 仓库根：优先脚本自身位置推导（scripts/ 的上级）——词表永远读脚本所在仓库，
# 在临时 git 仓库内自测（gatechecks 负控制自测）时不因 git toplevel 漂移。
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(SCRIPT_DIR)
# 扫描目标仓库：当前工作目录所在的 git 仓库（真实运行即本仓库，自测时是临时仓库）。
SCAN_ROOT = subprocess.run(
    ["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True, check=True
).stdout.strip()

# 扫描目录白名单：面向用户文案。业务术语密集目录（domains/、tools/ 等）不扫，
# 避免把「侵权」等法条/领域术语误报（见 tone-style-guide.md §4 分类说明）。
SCAN_DIRS = ("guardrails/", "server/", "agui/", "a2ui/", "pkg/i18n/")

# 文件级显式豁免（各带理由，见 docstring；新增豁免必须附理由注释）。
EXEMPT = {
    "guardrails/citation_table.go",
    "guardrails/consistency.go",
    "guardrails/fact_check.go",
}

# --- 词表解析（单源：docs/tone-style-guide.md §4） -------------------------------
section = re.search(
    r"## 4\. 禁用词表(.*?)(?=\n## )", open(f"{REPO_ROOT}/docs/tone-style-guide.md", encoding="utf-8").read(), re.S
)
if not section:
    print("✗ 未在 docs/tone-style-guide.md 找到「## 4. 禁用词表」段落", file=sys.stderr)
    sys.exit(2)

words = set()
for m in re.finditer(r"^\|\s*([^|]+?)\s*\|\s*([^|]*?)\s*\|$", section.group(1), re.M):
    cell = m.group(1).strip()
    if cell.startswith("禁用") or cell.replace("-", "") == "":  # 表头 / 表格分隔线 |---|
        continue
    # 去括号注释（如「不支持（生硬拒绝）」→「不支持」），再按顿号/斜杠拆词
    cell = re.sub(r"[（(].*?[)）]", "", cell)
    for part in re.split(r"[、/]", cell):
        part = part.strip()
        if len(part) >= 2:
            words.add(part)

# --- 扫描限定目录中被 git 跟踪的 Go 文件（跳过注释） ----------------------------
files = subprocess.run(
    ["git", "ls-files", "*.go"], capture_output=True, text=True, check=True, cwd=SCAN_ROOT
).stdout.splitlines()
files = [
    f for f in files
    if f.startswith(SCAN_DIRS) and f not in EXEMPT and not f.endswith("_test.go")
]

hits: dict[str, list[tuple[str, int, str]]] = {}  # word -> [(file, line, text)]
for f in files:
    for i, line in enumerate(open(f"{SCAN_ROOT}/{f}", encoding="utf-8", errors="replace"), 1):
        code = line.split("//")[0]  # 跳过代码注释：词表约束的是面向用户的文案
        for w in words:
            if w in code:
                hits.setdefault(w, []).append((f, i, line.strip()[:100]))

# --- 报告 ------------------------------------------------------------------------
print(f"禁用词表（docs/tone-style-guide.md §4 自动解析，共 {len(words)} 词）:")
print("  " + " / ".join(sorted(words)))
print()
print(
    f"扫描范围：{len(files)} 个 Go 文件（{', '.join(SCAN_DIRS)}，"
    f"豁免 {len(EXEMPT)} 个文件，跳过代码注释）"
)
total = sum(len(v) for v in hits.values())
print(f"存量命中：{len(hits)} 词命中，共 {total} 行")
print()

file_hits: dict[str, int] = {}
for v in hits.values():
    for f, _, _ in v:
        file_hits[f] = file_hits.get(f, 0) + 1

if hits:
    print("按词统计（命中行数 | 涉及文件数）:")
    for w in sorted(hits, key=lambda k: -len(hits[k])):
        print(f"  {len(hits[w]):5d} 行 | {len({f for f, _, _ in hits[w]}):3d} 文件 | {w}")
    print()
    print("命中文件 Top 15:")
    for f, n in sorted(file_hits.items(), key=lambda kv: -kv[1])[:15]:
        print(f"  {n:4d} 处 | {f}")
    print()
    print("示例（每词最多 3 行）:")
    shown = 0
    for w in sorted(hits):
        for f, ln, text in hits[w][:3]:
            print(f"  [{w}] {f}:{ln}: {text}")
        shown += 1
        if shown >= 10:
            print("  …")
            break
else:
    print("✓ 存量零命中")

sys.exit(1 if "--fail" in sys.argv and total > 0 else 0)
