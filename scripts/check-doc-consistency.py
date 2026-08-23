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
  5. go.work 模块集合 == {., ./tui, ./desktop}（tools 并入根模块后不得恢复）
  6. 维护文件清单不得残留 tools 独立模块表述（cd tools && / tools/go.sum /
     working-directory: tools / 四模块 / 三个子模块）
  7. 敏感路径清单双向一致：脚本 SENSITIVE_PATHS 数组 ↔ AGENTS.md 表相等；
     文档敏感路径段引用的路径 ⊆ 脚本集合（少列允许，多列不允许）
  8. .golangci-newcode.yml 与 .golangci.yml 生成式同步（--update 重新生成）
  9. workflows 不得硬编码 golangci 版本字面量（单源 .golangci-version）
  10. pre-commit 的 aichangelog files 过滤接线正确（防死规则复活）
  11. 包路径命名守卫（禁 common/utils/base 段）

用法:
  scripts/check-doc-consistency.py          # 退出码非 0 表示存在漂移
  scripts/check-doc-consistency.py --update # 检测到计数漂移时自动同步文档
"""

import glob
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

# --- 5. go.work 模块集合（tools 已并入根模块，2026-08-22 后应为三模块） ---------
go_work_text = open("go.work", encoding="utf-8").read()
go_work_modules: set[str] = set()
m = re.search(r"^use\s*\(([^)]*)\)", go_work_text, re.M)
if m:
    go_work_modules = {ln.strip() for ln in m.group(1).splitlines() if ln.strip()}
EXPECTED_MODULES = {".", "./tui", "./desktop"}
if go_work_modules != EXPECTED_MODULES:
    warn(
        f"go.work 模块集合 {sorted(go_work_modules)} ≠ 期望 {sorted(EXPECTED_MODULES)}"
        "（tools 已并入根模块，不得恢复独立 go.mod）"
    )

# --- 6. 维护文件禁 tools 独立模块残留（防四模块表述复发） -----------------------
MAINT_FILES = [
    "Makefile",
    ".github/workflows/ci.yml",
    ".github/workflows/ai-code-quality.yml",
    ".pre-commit-config.yaml",
    "scripts/precommit-golangci-lint.sh",
    "scripts/check-arch-boundaries.sh",
    "CONTRIBUTING.md",
    "CLAUDE.md",
    "AGENTS.md",
    "docs/GO-DEVELOPMENT-STANDARDS.md",
    "docs/developer-quickstart.md",
    ".github/PULL_REQUEST_TEMPLATE.md",
]
FORBIDDEN = [
    (r"cd tools &&", "tools 已并入根模块，不得再单独 cd tools 操作"),
    (r"tools/go\.sum", "tools 已无独立 go.mod/go.sum"),
    (r"working-directory:\s*tools", "CI 不得有 tools working-directory"),
    (r"四个模块", "应为三模块（root + tui + desktop）"),
    (r"3 个子模块", "应为 2 个子模块（root 含 tools/）"),
    (r"三个子模块", "应为 2 个子模块（root 含 tools/）"),
]
for _f in MAINT_FILES:
    try:
        _text = open(_f, encoding="utf-8").read()
    except OSError:
        warn(f"维护文件缺失: {_f}")
        continue
    for _pat, _why in FORBIDDEN:
        for _m in re.finditer(_pat, _text):
            _line = _text[: _m.start()].count("\n") + 1
            warn(f"{_f}:{_line} 残留 {_m.group(0)!r}（{_why}）")

# --- 7. 敏感路径清单双向一致（脚本 SENSITIVE_PATHS 数组为权威源） ----------------
_sp = open("scripts/check-sensitive-paths.sh", encoding="utf-8").read()
# 注意：数组内注释含半角括号（如 (resolvePathSandboxed)），故以 \n) 收尾匹配整个数组
_m = re.search(r"SENSITIVE_PATHS=\((.*?)\n\)", _sp, re.S)
_script_files = set(re.findall(r'"([^"]+)"', _m.group(1))) if _m else set()
_m = re.search(r"SENSITIVE_PATH_PREFIXES=\((.*?)\n\)", _sp, re.S)
_script_prefixes = set(re.findall(r'"([^"]+)"', _m.group(1))) if _m else set()
script_sensitive = _script_files | _script_prefixes

# AGENTS.md「安全敏感路径」段内 | `path` | 表格单元格 ↔ 脚本集合双向相等
_agents_section = re.search(r"## 安全敏感路径(.*?)(?=\n## )", agents_md, re.S)
agents_paths = (
    set(re.findall(r"^\|\s*`([^`]+)`\s*\|", _agents_section.group(1), re.M))
    if _agents_section
    else set()
)
if script_sensitive != agents_paths:
    for _p in sorted(script_sensitive - agents_paths):
        warn(f"AGENTS.md 敏感路径表缺少: {_p}")
    for _p in sorted(agents_paths - script_sensitive):
        warn(f"AGENTS.md 敏感路径表有多余（脚本未收录）: {_p}")


def check_sensitive_section(doc: str, section_re: re.Pattern, name: str) -> None:
    """文档敏感路径段引用的路径必须 ⊆ 脚本集合。

    少列允许（GO-STANDARDS/SECURITY.md 明示"代表性路径"），多列不允许——
    防文档新增了脚本没有的路径（权威源漂移）。
    """
    # 模式在 compile 时带 re.S（已编译 pattern 的 search 不接受 flags 参数，
    # 传 flags 会被当作 pos 整数参数导致从偏移处搜索而失配）
    _m = section_re.search(open(doc, encoding="utf-8").read())
    if not _m:
        warn(f"{name}: 未找到敏感路径段落（section_re 失配，检查文档结构）")
        return
    for _p in set(re.findall(r"`([^`]+)`", _m.group(1))):
        # 过滤文档自引用（脚本路径本身）与非路径 token
        if _p.startswith("scripts/") or "/" not in _p:
            continue
        if _p not in script_sensitive:
            warn(f"{name}: 引用了脚本未收录的敏感路径: {_p}")


check_sensitive_section(
    "CLAUDE.md",
    re.compile(r"### 敏感路径快速参考(.*?)(?=\n#{2,3} |\Z)", re.S),
    "CLAUDE.md",
)
check_sensitive_section(
    "docs/GO-DEVELOPMENT-STANDARDS.md",
    re.compile(r"### 12\.1 敏感路径(.*?)(?=\n#{2,3} |\Z)", re.S),
    "GO-STANDARDS §12.1",
)
check_sensitive_section(
    "SECURITY.md",
    re.compile(r"### 安全敏感路径(.*?)(?=\n#{2,3} |\Z)", re.S),
    "SECURITY.md",
)

# --- 8. .golangci-newcode.yml 生成式同步（杜绝人工漂移） ------------------------
NEWCODE_HEADER = (
    "# Mady Project - golangci-lint Configuration for NEW-CODE gate (v2 format)\n"
    "#\n"
    "# ⚠️ 由 scripts/check-doc-consistency.py --update 从 .golangci.yml 生成，禁止手改；\n"
    "#    修改根配置后运行 `python3 scripts/check-doc-consistency.py --update` 重新生成。\n"
    "# 与 .golangci.yml 相同，但额外启用 revive/exported（导出符号必须带注释）。\n"
    "# 用途：pre-commit 的 golangci-lint-newcode hook 以 `--new-from-rev=HEAD` 运行，\n"
    "# 只检查本次新增/修改代码，存量 exported 缺口不阻塞。\n"
)


def generate_newcode(root_text: str) -> str:
    """根配置 → newcode：在 revive empty-block 规则前插入 exported 规则。"""
    exported = "        - name: exported\n          severity: warning\n"
    anchor = "        - name: empty-block\n"
    if anchor not in root_text:
        raise ValueError("根配置缺少 empty-block revive 规则，无法定位插入点")
    return root_text.replace(anchor, exported + anchor, 1)


newcode_text = open(".golangci-newcode.yml", encoding="utf-8").read()
expected_newcode = NEWCODE_HEADER + generate_newcode(
    open(".golangci.yml", encoding="utf-8").read()
)
if newcode_text != expected_newcode:
    warn("golangci-newcode.yml 与根配置漂移（应运行 --update 重新生成，禁止手改）")
    if UPDATE:
        with open(".golangci-newcode.yml", "w", encoding="utf-8") as fh:
            fh.write(expected_newcode)
        updated_files.append(".golangci-newcode.yml")

# --- 9. workflows 不得硬编码 golangci 版本字面量（单源 .golangci-version） -------
for _wf in sorted(glob.glob(".github/workflows/*.yml")):
    _text = open(_wf, encoding="utf-8").read()
    for _m in re.finditer(r"version:\s*v?[0-9]+\.[0-9]+\.[0-9]+", _text):
        _line = _text[: _m.start()].count("\n") + 1
        warn(f"{_wf}:{_line} 硬编码版本字面量 {_m.group(0)!r}（版本单源 .golangci-version）")

# --- 10. pre-commit aichangelog files 接线（防死规则复活） -----------------------
_pc = open(".pre-commit-config.yaml", encoding="utf-8").read()
# 目标行是 yaml 字面正则：`[0-9]{4}-[0-9]{2}-[0-9]{2}\.md`，故字符类/量词/点
# 全部按字面转义匹配（\{4\} = 字面 {4}，\\.md = 字面 \.md）
if not re.search(
    r"files:\s*\^docs/decisions/ai-changelog/\[0-9\]\{4\}-\[0-9\]\{2\}-\[0-9\]\{2\}\\.md\$",
    _pc,
):
    warn("pre-commit aichangelog files 过滤未指向 ai-changelog 日期文件（死规则）")

# --- 11. 包路径命名守卫（禁 common/utils/base 段） -------------------------------
_list = subprocess.run(["go", "list", "./..."], capture_output=True, text=True)
if _list.returncode != 0:
    warn(f"go list ./... 失败（{_list.stderr.strip()}）——包名守卫未执行")
else:
    for _pkg in _list.stdout.splitlines():
        for _seg in ("/common/", "/utils/", "/base/"):
            if _seg in _pkg:
                warn(f"包路径含 {_seg.strip('/')} 段（命名规范守卫）: {_pkg}")

# --- 12. docs/adr 命名格式（NNN-<slug>.md） --------------------------------------
for _f in sorted(os.listdir("docs/adr")):
    # 编号为 3-4 位数字（存量 0001-0003 四位、006/007 三位），README 索引除外
    if _f.endswith(".md") and _f != "README.md" and not re.match(r"^\d{3,4}-.*\.md$", _f):
        warn(f"docs/adr/{_f} 不符合 NNN-<slug>.md 命名（数字前缀，见 docs/adr/README.md）")

# --- 13. docs/specs 四文件制（proposal/spec/design/tasks，显式例外清单） ----------
# 例外即理由记录：偏离目录必须在此登记并写明原因（元规则「例外显式」），
# 新增 spec 一律走完整四文件链（docs/specs/README.md）
SPEC_REQUIRED = ("01-proposal.md", "02-spec.md", "03-design.md", "04-tasks.md")
SPEC_DIR_EXCEPTIONS = {
    "enablement-a26.3": "26.3 充分公开任务拆解（内部工作文档，单文件 01-task-breakdown.md）",
    "plantask-introduction": "PlanTask 介绍性文档（plan.md，非功能设计链）",
    "prompt-templates-wiring": "提示词接线（design+tasks 两段，未走完整链）",
    "reexamination-request": "驳回复审请求工作流，仅 01-proposal（设计草案待 Sign-off，02/03/04 待补）",
    "tech-debt-fix": "技术债务修复，仅 04-tasks（已完成，阶段文档未补齐）",
}
SPEC_FILE_EXCEPTIONS = {
    "design-prior-art-retrieval-stage.md": "已上线功能的单文件设计文档",
    "design-rule-acquisition-stage.md": "已上线功能的单文件设计文档",
}
for _entry in sorted(os.listdir("docs/specs")):
    _spath = os.path.join("docs/specs", _entry)
    if os.path.isdir(_spath):
        if _entry in SPEC_DIR_EXCEPTIONS:
            continue
        for _f in SPEC_REQUIRED:
            if not os.path.exists(os.path.join(_spath, _f)):
                warn(f"docs/specs/{_entry}/ 缺少 {_f}（四文件制；例外需登记 SPEC_DIR_EXCEPTIONS 并写明理由）")
    elif _entry.endswith(".md") and _entry != "README.md" and _entry not in SPEC_FILE_EXCEPTIONS:
        warn(f"docs/specs/{_entry} 为散文件设计文档（未登记为例外，应并入目录或登记 SPEC_FILE_EXCEPTIONS）")

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
