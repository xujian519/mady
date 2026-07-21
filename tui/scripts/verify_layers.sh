#!/usr/bin/env bash
# verify_layers.sh — Check that tui/LAYERS.md directory listing matches
# the actual files on disk.
#
# Usage:
#   ./tui/scripts/verify_layers.sh          # exit 0 = in sync, exit 1 = drift
#   ./tui/scripts/verify_layers.sh --diff    # show drift in diff format
#
# Integrate into CI / Makefile:
#   verify-layers: ./tui/scripts/verify_layers.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TUI_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
LAYERS="$TUI_DIR/LAYERS.md"
MODE="${1:-check}"

if [ ! -f "$LAYERS" ]; then
  echo "ERROR: $LAYERS not found" >&2
  exit 2
fi

# --- Actual files on disk ---
# Get filenames only (no directory prefix), sorted, no test files
actual_names=$(cd "$TUI_DIR" && find . -type f -name "*.go" ! -name "*_test.go" \
  | sed 's|.*/||' | sort -u)

actual_count=$(echo "$actual_names" | grep -c . || true)

# --- Files listed in LAYERS.md ---
# Extract only from the ``` code block (between ``` markers),
# then find .go filenames and strip any directory prefix
listed_names=$(awk '/^```/{in_block=!in_block; next} in_block' "$LAYERS" \
  | grep -oE '[A-Za-z0-9_]+\.go' \
  | sort -u)

listed_count=$(echo "$listed_names" | grep -c . || true)

# --- Compare ---
missing=$(comm -23 <(echo "$actual_names") <(echo "$listed_names"))
phantom=$(comm -13 <(echo "$actual_names") <(echo "$listed_names"))

if [ -z "$missing" ] && [ -z "$phantom" ]; then
  echo "✅ LAYERS.md is in sync with tui/ directory ($actual_count files)"
  exit 0
fi

echo "❌ LAYERS.md is out of sync!"
echo "   Files on disk:      $actual_count"
echo "   Files in LAYERS.md:  $listed_count"
echo ""

if [ -n "$missing" ]; then
  echo "── Missing from LAYERS.md (on disk but not listed) ──"
  echo "$missing" | while read -r f; do
    [ -z "$f" ] && continue
    # Find the actual file to get line count
    fullpath=$(cd "$TUI_DIR" && find . -type f -name "$f" ! -name "*_test.go" | head -1)
    lines=$(wc -l < "$TUI_DIR/$fullpath" 2>/dev/null || echo "?")
    printf "  %-40s (%s lines)\n" "$f" "$lines"
  done
  echo ""
fi

if [ -n "$phantom" ]; then
  echo "── Phantom entries (in LAYERS.md but not on disk) ──"
  echo "$phantom" | while read -r f; do
    [ -z "$f" ] && continue
    echo "  $f"
  done
  echo ""
fi

if [ "$MODE" = "--diff" ]; then
  diff <(echo "$listed_names") <(echo "$actual_names") || true
fi

exit 1
