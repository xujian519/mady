#!/bin/bash
# check-aichangelog-format.sh
# 检查 ai-changelog 目录下日期文件新增条目是否包含必要字段。
# 作为 pre-commit hook 运行，仅检查已暂存（git add）的变更。
#
# 用法:
#   ./scripts/check-aichangelog-format.sh
#
# 门禁规则（只检查新添加的条目）：
#   必须包含：**背景** 和 **改动清单**
#   建议包含：**设计决策** 和 **影响**

set -euo pipefail

CHANGELOG_DIR="docs/decisions/ai-changelog"

# 检查 ai-changelog 目录下日期文件是否有暂存的变更
STAGED_FILES=$(git diff --cached --name-only | grep "^${CHANGELOG_DIR}/[0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}\.md$" || true)

if [ -z "$STAGED_FILES" ]; then
  exit 0
fi

ERRORS=""
for CHANGELOG in $STAGED_FILES; do
  # 只检查 added lines（以 + 开头）
  NEW_ENTRIES=$(git diff --cached "$CHANGELOG" | grep '^+## [a-z]' || true)

  if [ -z "$NEW_ENTRIES" ]; then
    continue  # 无新条目
  fi

  # 检查每个新条目后的字段
  CURRENT_ENTRY=""
  HAS_BACKGROUND=false
  HAS_CHANGELIST=false

  while IFS= read -r line; do
    # 检查是否是新增行（以 + 开头）且是条目标题
    if [[ "$line" =~ ^\+##[[:space:]][a-z] ]]; then
      # 遇到新条目，检查上一个条目
      if [ -n "$CURRENT_ENTRY" ]; then
        if [ "$HAS_BACKGROUND" = false ] || [ "$HAS_CHANGELIST" = false ]; then
          ERRORS="${ERRORS}  [$CHANGELOG] $CURRENT_ENTRY"
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
      ERRORS="${ERRORS}  [$CHANGELOG] $CURRENT_ENTRY"
      [ "$HAS_BACKGROUND" = false ] && ERRORS="${ERRORS} [缺少 **背景**]"
      [ "$HAS_CHANGELIST" = false ] && ERRORS="${ERRORS} [缺少 **改动清单**]"
      ERRORS="${ERRORS}\n"
    fi
  fi
done

if [ -n "$ERRORS" ]; then
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "❌ ai-changelog 新增条目缺少必要字段！"
  echo "    新条目必须包含 **背景** 和 **改动清单** 字段。"
  echo "    建议同时包含 **设计决策** 和 **影响** 字段。"
  echo "    请使用脚本追加: go run scripts/changelog/main.go"
  echo ""
  printf '%b' "$ERRORS"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  exit 1
fi

exit 0
