#!/usr/bin/env python3
"""文档-代码一致性校验（防漂移根因修复）。

校验 CLAUDE.md / AGENTS.md / CONTRIBUTING.md 中的关键声明与代码库实际状态
是否一致：
  1. 根模块 Go 文件计数（非测试/测试）与 CLAUDE.md、AGENTS.md 声明
  2. cmd/mady 子命令数量与文档声明（14 个）
  3. 文档目录树中引用的每个路径是否真实存在（解析嵌套层级）
  4. 内置领域 manifest 数量（应为 3 个）

用法: scripts/check-doc-consistency.py    # 退出码非 0 表示存在漂移
"""

import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
os.chdir(ROOT)

fail = 0


def warn(msg: str) -> None:
    global fail
    print(f"  ✗ {msg}")
    fail = 1


# --- 1. Go 文件计数 ----------------------------------------------------------
src_count = 0
test_count = 0
for dirpath, _, files in os.walk("."):
    if "/vendor/" in dirpath or dirpath.startswith("./vendor"):
        continue
    for f in files:
        if f.endswith(".go"):
            if f.endswith("_test.go"):
                test_count += 1
            else:
                src_count += 1

total = src_count + test_count
claude_md = open("CLAUDE.md", encoding="utf-8").read()
if total < 1400:
    warn(f"CLAUDE.md 声称 1400+ 个 Go 源文件，实际 {total}")

# 从 CLAUDE.md 提取声明计数（"~980 非测试 + ~450 测试"），±10 容差
m = re.search(r"~(\d+)\s*非测试\s*\+\s*~(\d+)\s*测试", claude_md)
if m:
    decl_src, decl_test = int(m.group(1)), int(m.group(2))
    if abs(src_count - decl_src) > 10:
        warn(f"CLAUDE.md 声明 ~{decl_src} 非测试文件，实际 {src_count}")
    if abs(test_count - decl_test) > 10:
        warn(f"CLAUDE.md 声明 ~{decl_test} 测试文件，实际 {test_count}")

# AGENTS.md 文件计数（若声明存在）— 非测试/测试/总数三向校验
m = re.search(
    r"(\d+)\s*个 Go 源文件\s*\((\d+)\s*非测试\s*\+\s*(\d+)\s*测试\)",
    open("AGENTS.md", encoding="utf-8").read(),
)
if m:
    decl_total, decl_src, decl_test = int(m.group(1)), int(m.group(2)), int(m.group(3))
    if abs(src_count - decl_src) > 5:
        warn(f"AGENTS.md 声明 {decl_src} 非测试文件，实际 {src_count}")
    if abs(test_count - decl_test) > 5:
        warn(f"AGENTS.md 声明 {decl_test} 测试文件，实际 {test_count}")
    if abs(total - decl_total) > 10:
        warn(f"AGENTS.md 声明 {decl_total} 个 Go 源文件，实际 {total}")

# --- 2. cmd/mady 子命令 ------------------------------------------------------
main_go = open("cmd/mady/main.go", encoding="utf-8").read()
# 提取所有 case 分支中的子命令名（一个 case 可含任意多个字符串字面量）
subcmd_names = set()
for m in re.finditer(r'^\tcase ((?:"[^"]+",?\s*)+):', main_go, re.M):
    for lit in re.findall(r'"([^"]+)"', m.group(1)):
        subcmd_names.add(lit)
subcmd_names.discard("-h")
subcmd_names.discard("--help")
subcmd_names.discard("help")
if len(subcmd_names) != 14:
    actual = sorted(subcmd_names)
    warn(f"cmd/mady 实际 {len(actual)} 个子命令（{actual}），文档声明 14 个")

# --- 3. 文档目录树路径存在性 --------------------------------------------------
# 通用树行正则：任意深度（`│   ` 缩进任意多级 + 分支符号）
TREE = re.compile(r"^((?:│   )*)(├──|└──)\s+([^\s#]+)")


def check_doc_tree(doc_path: str) -> None:
    """解析文档目录树，构建完整路径并检查存在性。"""
    stack: list[str] = []  # 当前路径栈
    for line in open(doc_path, encoding="utf-8"):
        line = line.rstrip("\n")
        m = TREE.match(line)
        if not m:
            continue
        indent, _, name = m.group(1), m.group(2), m.group(3).rstrip("/,")
        if "*" in name:  # 通配符路径（如 browser_*.go）跳过
            continue
        depth = indent.count("│") + 1
        while len(stack) >= depth:
            stack.pop()
        full = os.path.join(stack[-1], name) if stack else name
        if not os.path.exists(full):
            warn(f"{doc_path} 引用的路径不存在: {full}")
        stack.append(full)


check_doc_tree("CLAUDE.md")
check_doc_tree("CONTRIBUTING.md")

# --- 4. 内置 manifest 数量 ---------------------------------------------------
manifest_count = len(
    [f for f in os.listdir("agentcore/manifests") if f.endswith(".json")]
)
if manifest_count != 3:
    warn(f"agentcore/manifests/ 实际 {manifest_count} 个 manifest（文档声明 3 个）")

if fail == 0:
    print(
        f"✓ 文档一致性检查通过（src={src_count} test={test_count}, "
        f"子命令={len(subcmd_names)}, manifests={manifest_count}）"
    )
else:
    print("✗ 文档一致性检查失败 — 请同步更新 CLAUDE.md / AGENTS.md / CONTRIBUTING.md")
sys.exit(1 if fail else 0)
