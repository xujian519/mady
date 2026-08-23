#!/usr/bin/env python3
"""tone-style 禁用词存量扫描（先评估后决策，未纳入门禁）。

扫描被 git 跟踪的 Go 源文件中面向用户文案的禁用/慎用词（绝对、一定、百分百、
保证、从不、无法处理、不支持、冲突、侵权等），产出存量命中报告供决策
（tone-style 门禁是否上线、词表是否调整）。

词表自动解析自 docs/tone-style-guide.md「## 4. 禁用词表」表格——单源解析，
禁止在本脚本内手写词表（防词表与文档漂移，见 AGENTS.md「规范元规则」三件套）。

用法:
  scripts/check-tone-words.py           # 只报告命中统计（exit 0）
  scripts/check-tone-words.py --fail    # 命中即 exit 1（将来上门禁时用）
"""

import re
import subprocess
import sys

ROOT = subprocess.run(
    ["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True, check=True
).stdout.strip()

# --- 词表解析（单源：docs/tone-style-guide.md §4） -------------------------------
section = re.search(
    r"## 4\. 禁用词表(.*?)(?=\n## )", open(f"{ROOT}/docs/tone-style-guide.md", encoding="utf-8").read(), re.S
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

# --- 扫描被 git 跟踪的 Go 文件 ---------------------------------------------------
files = subprocess.run(
    ["git", "ls-files", "*.go"], capture_output=True, text=True, check=True, cwd=ROOT
).stdout.splitlines()

hits: dict[str, list[tuple[str, int, str]]] = {}  # word -> [(file, line, text)]
for f in files:
    for i, line in enumerate(open(f"{ROOT}/{f}", encoding="utf-8", errors="replace"), 1):
        for w in words:
            if w in line:
                hits.setdefault(w, []).append((f, i, line.strip()[:100]))

# --- 报告 ------------------------------------------------------------------------
print(f"禁用词表（docs/tone-style-guide.md §4 自动解析，共 {len(words)} 词）:")
print("  " + " / ".join(sorted(words)))
print()
total = sum(len(v) for v in hits.values())
print(f"存量命中：{len(hits)} 词命中，共 {total} 行（git 跟踪 Go 文件 {len(files)} 个）")
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
