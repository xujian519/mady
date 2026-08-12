#!/usr/bin/env python3
"""文档-代码一致性校验（防漂移根因修复）。

校验 CLAUDE.md / AGENTS.md / CONTRIBUTING.md 中的关键声明与代码库实际状态
是否一致：
  1. 被 git 跟踪的 Go 文件计数（非测试/测试）与 CLAUDE.md、AGENTS.md 声明
     —— 使用 `git ls-files` 口径，与版本库一致、结果可复现，天然排除
     node_modules、vendor、构建产物等未跟踪/被忽略目录
  2. cmd/mady 子命令数量与文档声明（17 个）
  3. 文档目录树中引用的每个路径是否真实存在（解析嵌套层级）
  4. 内置领域 manifest 数量（应为 3 个）

用法:
  scripts/check-doc-consistency.py          # 退出码非 0 表示存在漂移
  scripts/check-doc-consistency.py --update # 检测到计数漂移时自动同步文档
"""

import os
import re
import subprocess
import sys
from datetime import date

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
os.chdir(ROOT)

UPDATE = "--update" in sys.argv
fail = 0
updated_files: list[str] = []


def warn(msg: str) -> None:
    global fail
    print(f"  ✗ {msg}")
    fail = 1


def sync_file(path: str, subs: list[tuple[re.Pattern, str]]) -> None:
    """按 subs 逐个替换 path 中首个匹配；发生变更则写回并记录。"""
    text = open(path, encoding="utf-8").read()
    new = text
    for pat, repl in subs:
        new = pat.sub(repl, new, count=1)
    if new != text:
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(new)
        updated_files.append(path)


# --- 1. Go 文件计数（git 跟踪口径） ------------------------------------------
def get_git_counts() -> tuple[int, int]:
    """统计被 git 跟踪的 Go 文件（返回非测试数、测试数）。"""
    out = subprocess.run(
        ["git", "ls-files"], capture_output=True, text=True, check=True
    ).stdout.splitlines()
    src = test = 0
    for f in out:
        if f.endswith("_test.go"):
            test += 1
        elif f.endswith(".go"):
            src += 1
    return src, test


src_count, test_count = get_git_counts()
total = src_count + test_count
hundreds = total // 100 * 100  # "1500+" 这类百位下限声明应取的值

claude_md = open("CLAUDE.md", encoding="utf-8").read()

# 百位下限声明（"1500+ 个 Go 源文件"）—— 声明是下限，实际不得低于它
m = re.search(r"(\d+)\+\s*个 Go 源文件", claude_md)
if m:
    if total < int(m.group(1)):
        warn(f"CLAUDE.md 声称 {m.group(1)}+ 个 Go 源文件，实际 {total}")
        if UPDATE:
            sync_file(
                "CLAUDE.md",
                [(re.compile(r"\d+\+\s*个 Go 源文件"), f"{hundreds}+ 个 Go 源文件")],
            )
elif total < 1400:  # 声明缺失时的兜底下限
    warn(f"CLAUDE.md 声称 1400+ 个 Go 源文件，实际 {total}")

# 精确计数声明（"~980 非测试 + ~450 测试"），±10 容差
m = re.search(r"~(\d+)\s*非测试\s*\+\s*~(\d+)\s*测试", claude_md)
if m:
    decl_src, decl_test = int(m.group(1)), int(m.group(2))
    src_drift = abs(src_count - decl_src) > 10
    test_drift = abs(test_count - decl_test) > 10
    if src_drift:
        warn(f"CLAUDE.md 声明 ~{decl_src} 非测试文件，实际 {src_count}")
    if test_drift:
        warn(f"CLAUDE.md 声明 ~{decl_test} 测试文件，实际 {test_count}")
    if (src_drift or test_drift) and UPDATE:
        sync_file(
            "CLAUDE.md",
            [
                (
                    re.compile(r"~\d+\s*非测试\s*\+\s*~\d+\s*测试"),
                    f"~{src_count} 非测试 + ~{test_count} 测试",
                )
            ],
        )

# AGENTS.md 文件计数（若声明存在）— 非测试/测试/总数三向校验。
# 注意：AGENTS.md 使用全角括号（），正则必须匹配全角字符，
# 历史版本误用半角 \(\) 导致该检查从未生效。
agents_md = open("AGENTS.md", encoding="utf-8").read()
m = re.search(
    r"(\d+)\s*个 Go 源文件\s*（(\d+)\s*非测试\s*\+\s*(\d+)\s*测试）",
    agents_md,
)
if m:
    decl_total, decl_src, decl_test = int(m.group(1)), int(m.group(2)), int(m.group(3))
    drift = False
    if abs(src_count - decl_src) > 5:
        warn(f"AGENTS.md 声明 {decl_src} 非测试文件，实际 {src_count}")
        drift = True
    if abs(test_count - decl_test) > 5:
        warn(f"AGENTS.md 声明 {decl_test} 测试文件，实际 {test_count}")
        drift = True
    if abs(total - decl_total) > 10:
        warn(f"AGENTS.md 声明 {decl_total} 个 Go 源文件，实际 {total}")
        drift = True
    if drift and UPDATE:
        sync_file(
            "AGENTS.md",
            [
                (
                    re.compile(
                        r"\d+\s*个 Go 源文件\s*（\d+\s*非测试\s*\+\s*\d+\s*测试）"
                    ),
                    f"{total} 个 Go 源文件（{src_count} 非测试 + {test_count} 测试）",
                )
            ],
        )
        # 计数被同步时，一并刷新"文件计数更新时间"标记
        sync_file(
            "AGENTS.md",
            [
                (
                    re.compile(r"文件计数更新时间：\d{4}-\d{2}-\d{2}"),
                    f"文件计数更新时间：{date.today().isoformat()}",
                )
            ],
        )

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
if len(subcmd_names) != 17:
    actual = sorted(subcmd_names)
    warn(f"cmd/mady 实际 {len(actual)} 个子命令（{actual}），文档声明 17 个")

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
    if UPDATE and updated_files:
        print(
            "  ℹ 已自动同步计数: "
            + ", ".join(updated_files)
            + " — 请重新运行 doc-check 确认"
        )
    print("✗ 文档一致性检查失败 — 请同步更新文档或运行 scripts/check-doc-consistency.py --update")
sys.exit(1 if fail else 0)
