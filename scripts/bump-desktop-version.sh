#!/usr/bin/env bash
set -euo pipefail

# bump-desktop-version.sh — 桌面端版本单点维护（调研报告 R-P1-5）。
#
# 权威源 = desktop/wails.json 的 info.productVersion；本脚本联动所有派生位置：
#   - desktop/app_update.go 的 desktopVersion 默认值（ldflags 未注入时的回退）
#   - desktop/frontend/package.json 的 version（npm/pnpm 元数据）
#
# 构建期 Info.plist 由 Wails 模板从 wails.json 生成（见 desktop/build/darwin/Info.plist
# 的 {{.Info.ProductVersion}}），无需手动改；发布版本经 ldflags 注入
# `-X main.desktopVersion=$(VERSION)`（Makefile DESKTOP_LDFLAGS / CI desktop-release.yml）。
#
# 用法：./scripts/bump-desktop-version.sh 0.2.0
# 发版顺序：改 wails.json（或直接跑本脚本）→ make desktop-dmg → 打 desktop-v* tag。

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NEW="${1:-}"

if [[ -z "$NEW" ]]; then
  echo "用法: $0 <semver>  （如 0.2.0 或 v0.2.0）" >&2
  exit 1
fi

if [[ ! "$NEW" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "错误: 版本号格式无效（期望 SemVer，如 0.2.0 或 v0.2.0）" >&2
  exit 1
fi

NEW_NO_V="${NEW#v}"

WAILS_JSON="$ROOT/desktop/wails.json"
APP_UPDATE="$ROOT/desktop/app_update.go"
PKG_JSON="$ROOT/desktop/frontend/package.json"

if ! grep -q '"productVersion"' "$WAILS_JSON"; then
  echo "错误: $WAILS_JSON 中未找到 productVersion" >&2
  exit 1
fi

# macOS 与 Linux 通用：GNU sed 的 -i 与 BSD sed 的 -i '' 不兼容，统一用 .bak 中转。
sed -i.bak "s/\(\"productVersion\"[[:space:]]*:[[:space:]]*\)\"[^\"]*\"/\1\"$NEW_NO_V\"/" "$WAILS_JSON"
rm -f "$WAILS_JSON.bak"

sed -i.bak "s/\(var desktopVersion = \)\"[^\"]*\"/\1\"$NEW_NO_V\"/" "$APP_UPDATE"
rm -f "$APP_UPDATE.bak"

if grep -q '"version"' "$PKG_JSON"; then
  sed -i.bak "s/\(\"version\"[[:space:]]*:[[:space:]]*\)\"[^\"]*\"/\1\"$NEW_NO_V\"/" "$PKG_JSON"
  rm -f "$PKG_JSON.bak"
fi

# 替换校验：sed 匹配失败时静默返回 0，这里逐文件显式确认新版本已落到目标，
# 避免 JSON 被格式化（如多行展开）后脚本「假装成功」。
if ! grep -Eq "\"productVersion\"[[:space:]]*:[[:space:]]*\"$NEW_NO_V\"" "$WAILS_JSON"; then
  echo "错误: wails.json 版本替换未生效，请检查文件格式" >&2
  exit 1
fi
if ! grep -Eq "var desktopVersion = \"$NEW_NO_V\"" "$APP_UPDATE"; then
  echo "错误: app_update.go 版本替换未生效，请检查文件格式" >&2
  exit 1
fi
if grep -q '"version"' "$PKG_JSON" && ! grep -Eq "\"version\"[[:space:]]*:[[:space:]]*\"$NEW_NO_V\"" "$PKG_JSON"; then
  echo "错误: frontend/package.json 版本替换未生效，请检查文件格式" >&2
  exit 1
fi

echo "✅ 桌面端版本已统一为 ${NEW_NO_V}：wails.json / app_update.go / frontend/package.json"
echo "   后续：make desktop-dmg && make desktop-notarize，然后打 tag desktop-v${NEW_NO_V} 触发发布流水线"
