#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""检查 Go 源文件中的注释拼接残留（纯搬移重构的廉价回归防护）。

纯搬移拆分（大文件按职责拆文件）最常见的副作用是注释被切在文件边界：
  1. 同一句注释在拼接处整行重复两次
  2. 句子断在半截（英文断在 "that the" 等介词/连词后）

用法：
  scripts/check-comment-seams.sh            # 检查全部 git 跟踪 .go 文件
  scripts/check-comment-seams.sh [files...] # 只检查指定文件（pre-commit 传入）

退出码：发现疑似残留时 1（阻塞提交），否则 0。

注意：用 python3 而非 awk —— macOS BSD awk 的字符串比较对多字节 UTF-8
不可靠（曾把 "测试用例" 与 "────" 判为相等）。
"""

import re
import subprocess
import sys

# 断片特征：英文断在 "the/a/an" 前的介词/连词（高置信度）。
FRAGMENT_RE = re.compile(
    r"(that|and|with|of|for|to|in|from|by|on) (the|a|an)$"
    r"|(to be|is a|are a|was a|were a)$"
)
DECOR_RE = re.compile(r"^[-=*#/]+$")
COMMENT_RE = re.compile(r"^[ \t]*//")


def check_file(path: str) -> list[str]:
    """返回该文件的疑似残留列表（file:line: message）。"""
    findings: list[str] = []
    try:
        with open(path, encoding="utf-8") as fh:
            lines = fh.readlines()
    except (OSError, UnicodeDecodeError):
        return findings

    prev_text = ""
    in_block = False
    last_text = ""
    last_lineno = 0

    for lineno, raw in enumerate(lines, 1):
        line = raw.rstrip("\n")
        if COMMENT_RE.match(line):
            text = re.sub(r"^[ \t]*//[ \t]?", "", line)
            if len(text) >= 20 and text == prev_text:
                findings.append(f"{path}:{lineno}: 重复注释行: {text}")
            prev_text = text
            in_block = True
            last_text = text
            last_lineno = lineno
        else:
            if in_block and not DECOR_RE.match(last_text):
                if FRAGMENT_RE.search(last_text):
                    findings.append(f"{path}:{last_lineno}: 疑似注释断片: {last_text}")
            in_block = False
            prev_text = ""

    if in_block and not DECOR_RE.match(last_text):
        if FRAGMENT_RE.search(last_text):
            findings.append(f"{path}:{last_lineno}: 疑似注释断片: {last_text}")

    return findings


def main() -> int:
    if len(sys.argv) > 1:
        files = [a for a in sys.argv[1:] if a]
    else:
        files = subprocess.check_output(
            ["git", "ls-files", "*.go"], text=True
        ).splitlines()

    hits = 0
    for f in files:
        for msg in check_file(f):
            print(msg)
            hits += 1

    if hits:
        print(
            "\ncheck-comment-seams: 发现疑似注释拼接残留，请检查上述位置。",
            file=sys.stderr,
        )
        print(
            "常见来源：大文件拆分时注释被切在文件边界。"
            "修复：把完整注释合并到所属声明处。",
            file=sys.stderr,
        )
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
