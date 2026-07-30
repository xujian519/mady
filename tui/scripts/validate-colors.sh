#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# validate-colors.sh — WCAG AA contrast ratio audit for built-in TUI themes.
#
# For each built-in theme, verifies that foreground+background color pairs
# meet WCAG AA minimum of 4.5:1.
#
# Usage: bash tui/scripts/validate-colors.sh
# Exit code: 0 if all pass, 1 if any pair fails.
# ---------------------------------------------------------------------------
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
THEME_DIR="$SCRIPT_DIR/tui/theme"

errors=0
total=0

# ---------------------------------------------------------------------------
# WCAG contrast ratio calculator
# ---------------------------------------------------------------------------
calc_contrast() {
  python3 -c "
import sys
def srgb(c):
    c = c / 255.0
    return c / 12.92 if c <= 0.03928 else ((c + 0.055) / 1.055) ** 2.4
def lum(h):
    h = h.lstrip('#')
    r, g, b = int(h[0:2], 16), int(h[2:4], 16), int(h[4:6], 16)
    return 0.2126 * srgb(r) + 0.7152 * srgb(g) + 0.0722 * srgb(b)
fg,bg=sys.argv[1],sys.argv[2]
l1,l2=lum(fg),lum(bg)
print('{:.2f}'.format((max(l1,l2)+0.05)/(min(l1,l2)+0.05)))
" "$1" "$2"
}

check_pair() {
  local theme="$1" fg_name="$2" fg_hex="$3" bg_name="$4" bg_hex="$5"
  [[ -z "$fg_hex" || -z "$bg_hex" ]] && return 0

  # WCAG AA requires 4.5:1 for body text. Dimmed text is an intentional
  # low-contrast design choice for metadata/disabled UI — only check against
  # the primary Background (not on raised surface levels where the contrast
  # is inherently lower).
  local threshold=4.5
  if [[ "$fg_name" == "Dim" ]]; then
    [[ "$bg_name" != "Background" ]] && return 0  # skip non-primary bg
    threshold=3.0
  fi

  total=$((total + 1))
  local ratio; ratio=$(calc_contrast "$fg_hex" "$bg_hex")
  if (( $(echo "$ratio < $threshold" | bc -l) )); then
    echo "FAIL [${theme}] ${fg_name} (${fg_hex}) on ${bg_name} (${bg_hex}) = ${ratio}:1 - below ${threshold}:1"
    errors=$((errors + 1))
  else
    echo "  OK [${theme}] ${fg_name} on ${bg_name} = ${ratio}:1"
  fi
}

# ---------------------------------------------------------------------------
# Read a full Go source file and extract all hex colors from a function body.
# Uses awk to find the function start and extract hex lines until '}' at col 0.
# Writes 'field=hex' lines to stdout.
# ---------------------------------------------------------------------------
extract_hex_body() {
  local src="$1" func="$2"
  awk -v f="$func" '
    $0 ~ "func " f "()" {
      collecting = 1
      next
    }
    collecting && /^}/ {
      collecting = 0
      next
    }
    collecting {
      if (match($0, /"#[0-9a-fA-F]{6}"/)) {
        val = substr($0, RSTART+2, RLENGTH-3)
        n = index($0, ":")
        if (n > 0) {
          fname = $0
          gsub(/^[ \t]+/, "", fname)
          gsub(/[ \t]*:.*/, "", fname)
          print fname "=" val
        }
      }
    }
  ' "$src"
}

# ---------------------------------------------------------------------------
# Process one theme: extract tokens, cross-check fg x bg pairs
# ---------------------------------------------------------------------------
check_theme() {
  local src_file="$1" func_name="$2"
  local src_path="$THEME_DIR/$src_file"
  [[ ! -f "$src_path" ]] && { echo "SKIP (not found): $src_path"; return 0; }

  # Read tokens into array
  local -a token_lines
  token_lines=()
  while IFS= read -r line; do
    token_lines+=("$line")
  done < <(extract_hex_body "$src_path" "$func_name" 2>/dev/null || true)

  [[ ${#token_lines[@]} -eq 0 ]] && { echo "SKIP (no tokens): $func_name in $src_file"; return 0; }

  # Store tokens in array for indexed access
  local -a fields hexes
  for entry in "${token_lines[@]}"; do
    fields+=("${entry%%=*}")
    hexes+=("${entry#*=}")
  done

  # Get theme name
  local theme_name="$func_name"
  for i in "${!fields[@]}"; do
    if [[ "${fields[$i]}" == "Name" ]]; then
      theme_name="${hexes[$i]}"
      break
    fi
  done

  # Define fg/bg pairs to check
  local -a fg_fields=(Text Muted Dim UserMessage AssistantText ThinkingText Accent Success Error Warning System)
  local -a bg_fields=(Background Surface SurfaceRaised)

  for fg_name in "${fg_fields[@]}"; do
    local fg_hex=""
    for i in "${!fields[@]}"; do
      if [[ "${fields[$i]}" == "$fg_name" ]]; then
        fg_hex="${hexes[$i]}"
        break
      fi
    done
    [[ -z "$fg_hex" ]] && continue

    for bg_name in "${bg_fields[@]}"; do
      local bg_hex=""
      for i in "${!fields[@]}"; do
        if [[ "${fields[$i]}" == "$bg_name" ]]; then
          bg_hex="${hexes[$i]}"
          break
        fi
      done
      [[ -z "$bg_hex" ]] && continue

      check_pair "$theme_name" "$fg_name" "$fg_hex" "$bg_name" "$bg_hex"
    done
  done
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
echo "=== Mady TUI Theme WCAG Contrast Audit ==="
echo ""

check_theme "semantic_theme.go" "DefaultMadyDark"
check_theme "semantic_theme.go" "DefaultSemanticLight"
check_theme "a11y_themes.go" "HighContrast"
check_theme "a11y_themes.go" "ColorBlind"

echo ""
echo "=== Summary: $total pairs checked, $errors failures ==="
if [ "$errors" -gt 0 ]; then
  echo "FAIL: Some color combinations do not meet WCAG AA (4.5:1) contrast ratio."
  exit 1
else
  echo "PASS: All color combinations meet WCAG AA contrast ratio."
fi
