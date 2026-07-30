#!/bin/bash
# check-aichangelog-format.sh
# 检查 AI_CHANGELOG.md 的新增条目是否包含必要字段。
# 作为 pre-commit hook 运行，仅检查已暂存（git add）的变更。
#
# 用法:
#   ./scripts/check-aichangelog-format.sh
#
# 门禁规则（只检查新添加的条目）：
#   必须包含：**背景** 和 **改动清单**
#   建议包含：**设计决策** 和 **影响**

set -euo pipefail

CHANGELOG="docs/decisions/AI_CHANGELOG.md"

# 检查 AI_CHANGELOG.md 是否有暂存的变更
if ! git diff --cached --name-only | grep -q -Fx "$CHANGELOG"; then
  exit 0
fi

# 从暂存 diff 中提取新增的条目标题和紧随其后的行
# 格式：## YYYY-MM-DD: ...
# 只检查 added lines（以 + 开头）
NEW_ENTRIES=$(git diff --cached "$CHANGELOG" | grep '^+## [0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}' || true)

if [ -z "$NEW_ENTRIES" ]; then
  exit 0  # 无新条目
fi

# 检查每个新条目后的字段 — 提取 diff 中每个新条目块
# 通过识别 added lines 模式来分段
ERRORS=""
CURRENT_ENTRY=""
HAS_BACKGROUND=false
HAS_CHANGELIST=false

while IFS= read -r line; do
  # 检查是否是新增行（以 + 开头）
  if [[ "$line" =~ ^\+##[[:space:]] ]]; then
    # 遇到新条目，检查上一个条目
    if [ -n "$CURRENT_ENTRY" ]; then
      if [ "$HAS_BACKGROUND" = false ] || [ "$HAS_CHANGELIST" = false ]; then
        ERRORS="${ERRORS}  - $CURRENT_ENTRY"
        [ "$HAS_BACKGROUND" = false ] && ERRORS="${ERRORS} [缺少 **背景**]"
        [ "$HAS_CHANGELIST" = false ] && ERRORS="${ERRORS} [缺少 **改动清单**]"
        ERRORS="${ERRORS}\n"
      fi
    fi
    # 重置状态
    CURRENT_ENTRY=$(echo "$line" | sed 's/^+//')
    HAS_BACKGROUND=false
    HAS_CHANGELIST=false
  elif echo "$line" | grep -q '^\+\*\*背景\*\*'; then
    HAS_BACKGROUND=true
  elif echo "$line" | grep -q '^\+\*\*改动清单\*\*'; then
    HAS_CHANGELIST=true
  fi
done < <(git diff --cached "$CHANGELOG")

# 检查最后一个条目
if [ -n "$CURRENT_ENTRY" ]; then
  if [ "$HAS_BACKGROUND" = false ] || [ "$HAS_CHANGELIST" = false ]; then
    ERRORS="${ERRORS}  - $CURRENT_ENTRY"
    [ "$HAS_BACKGROUND" = false ] && ERRORS="${ERRORS} [缺少 **背景**]"
    [ "$HAS_CHANGELIST" = false ] && ERRORS="${ERRORS} [缺少 **改动清单**]"
    ERRORS="${ERRORS}\n"
  fi
fi

if [ -n "$ERRORS" ]; then
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "❌ AI_CHANGELOG.md 新增条目缺少必要字段！"
  echo "    新条目必须包含 **背景** 和 **改动清单** 字段。"
  echo "    建议同时包含 **设计决策** 和 **影响** 字段。"
  echo ""
  printf '%b' "$ERRORS"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  exit 1
fi

exit 0
