#!/bin/bash
# check-sensitive-paths.sh
# 检查当前 diff 是否涉及安全敏感路径，以及提交是否含 AI 协助标记。
# 配合 pre-commit hook 和 CI Workflow 使用。
#
# 用法:
#   ./scripts/check-sensitive-paths.sh [--ci-base <ref>]
#     --ci-base <ref>  在 CI 中与指定 base ref 对比 (如 origin/main)
#     无参数时检测 git diff --cached (pre-commit)

set -euo pipefail

# 安全敏感路径列表（基于 Mady 项目安全红线分析）
SENSITIVE_PATHS=(
  "agentcore/handoff.go"          # 交接白名单校验 (isHandoffAllowed)
  "guardrails/levels.go"          # 护栏等级枚举 (Light/Standard/Strict)
  "domains/router.go"             # 路由白名单 AllowedSources
  "domains/patent.go"             # 动态 WorkingDir (BuildProjectAgent)
  "domains/approval.go"           # ApprovalGate 生命周期钩子
  "tools/path.go"                 # 文件系统沙箱隔离 (resolvePathSandboxed)
  "tools/tools.go"                # 工具能力门控 (ExtensionConfig)
  "agentcore/manifest.go"         # Manifest 校验规则
  "domains/project.go"            # ValidateProjectPath 路径校验
  "tools/bash.go"                 # Bash 工具 (非沙箱模式)
)

HAS_SENSITIVE_CHANGES=false
HAS_AI_COAUTHOR=false
CHANGED_FILES=""

# 解析参数
BASE_REF=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --ci-base)
      BASE_REF="$2"
      if [ -z "$BASE_REF" ]; then
        echo "Error: --ci-base requires a non-empty value" >&2
        exit 1
      fi
      shift 2
      ;;
    *)
      echo "Unknown option: $1" >&2
      echo "Usage: $0 [--ci-base <ref>]" >&2
      exit 1
      ;;
  esac
done

# 获取变更文件列表
if [ -n "$BASE_REF" ]; then
  CHANGED_FILES=$(git diff --name-only "$BASE_REF"...HEAD 2>/dev/null) || {
    echo "Error: git diff failed for ref $BASE_REF" >&2
    exit 1
  }
else
  CHANGED_FILES=$(git diff --cached --name-only 2>/dev/null) || {
    echo "Error: git diff --cached failed" >&2
    exit 1
  }
fi

if [ -z "$CHANGED_FILES" ]; then
  echo "没有检测到变更文件，跳过敏感路径检查。"
  echo "提示：如尚未 git add，请先暂存变更后重新运行。"
  exit 0
fi

# 检查是否修改了敏感路径（使用 -Fx 进行整行精确匹配，避免子串误报）
SENSITIVE_HITS=""
for path in "${SENSITIVE_PATHS[@]}"; do
  if echo "$CHANGED_FILES" | grep -q -Fx "$path"; then
    HAS_SENSITIVE_CHANGES=true
    SENSITIVE_HITS="${SENSITIVE_HITS}  - $path\n"
  fi
done

# 检查提交信息是否含 AI 协助标记
# 在 --ci-base 模式下检查范围中的所有 commit，而非仅 HEAD
AI_DETECT_REGEX="co-authored-by.*(claude|ai|copilot|codex|gemini)"
if [ -n "$BASE_REF" ]; then
  if git log --format=%B "$BASE_REF"...HEAD 2>/dev/null | grep -qiE "$AI_DETECT_REGEX"; then
    HAS_AI_COAUTHOR=true
  fi
else
  if git log --format=%B -n 1 HEAD 2>/dev/null | grep -qiE "$AI_DETECT_REGEX"; then
    HAS_AI_COAUTHOR=true
  fi
fi

# --- 输出结果 ---

if [ "$HAS_SENSITIVE_CHANGES" = true ]; then
  echo ""
  echo "⚠️  检测到安全敏感路径变更:"
  printf '%b' "$SENSITIVE_HITS"
fi

if [ "$HAS_AI_COAUTHOR" = true ]; then
  echo ""
  echo "🤖 检测到 AI 协助标记 (Co-authored-by)"
fi

# 关键判断：AI 参与 + 敏感路径 = 阻塞
if [ "$HAS_SENSITIVE_CHANGES" = true ] && [ "$HAS_AI_COAUTHOR" = true ]; then
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "🚨  AI 参与的变更涉及安全敏感路径！"
  echo "    禁止未经人工审阅直接合入。"
  echo ""
  echo "    请完成以下步骤后重新提交："
  echo "    1. 在 PR 描述中勾选 '涉红线变更'"
  echo "    2. 至少一位人类维护者完成代码审查"
  echo "    3. 在 AI_CHANGELOG.md 中记录本次变更决策"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  exit 1
fi

if [ "$HAS_SENSITIVE_CHANGES" = true ]; then
  echo ""
  echo "💡 提示：此变更涉及安全敏感路径，建议在 PR 中标注并请求人工审查。"
fi

if [ "$HAS_AI_COAUTHOR" = true ]; then
  echo ""
  echo "💡 提示：AI 协助标记已识别，请在 PR 模板中说明 AI 参与级别。"
fi

exit 0
