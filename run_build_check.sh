#!/bin/bash
set +e

echo "=== 1. go work ===="
cat /Users/xujian/projects/Mady/go.work

echo ""
echo "=== 2. Build: root module ==="
cd /Users/xujian/projects/Mady && go build ./... 2>&1
ROOT_BUILD=$?
echo "Exit code: $ROOT_BUILD"

echo ""
echo "=== 3. Build: tui ==="
cd /Users/xujian/projects/Mady/tui && go build ./... 2>&1
TUI_BUILD=$?
echo "Exit code: $TUI_BUILD"

echo ""
echo "=== 4. Build: tools ==="
cd /Users/xujian/projects/Mady/tools && go build ./... 2>&1
TOOLS_BUILD=$?
echo "Exit code: $TOOLS_BUILD"

echo ""
echo "=== 5. Build: desktop ==="
cd /Users/xujian/projects/Mady/desktop && go build ./... 2>&1
DESKTOP_BUILD=$?
echo "Exit code: $DESKTOP_BUILD"

echo ""
echo "=== 6. Vet: root module ==="
cd /Users/xujian/projects/Mady && go vet ./... 2>&1
ROOT_VET=$?
echo "Exit code: $ROOT_VET"

echo ""
echo "=== 7. Vet: tui ==="
cd /Users/xujian/projects/Mady/tui && go vet ./... 2>&1
TUI_VET=$?
echo "Exit code: $TUI_VET"

echo ""
echo "=== 8. Vet: tools ==="
cd /Users/xujian/projects/Mady/tools && go vet ./... 2>&1
TOOLS_VET=$?
echo "Exit code: $TOOLS_VET"

echo ""
echo "=== 9. Vet: desktop ==="
cd /Users/xujian/projects/Mady/desktop && go vet ./... 2>&1
DESKTOP_VET=$?
echo "Exit code: $DESKTOP_VET"

echo ""
echo "=== Summary ==="
echo "Root build: $ROOT_BUILD | Root vet: $ROOT_VET"
echo "Tools build: $TOOLS_BUILD | Tools vet: $TOOLS_VET"
echo "TUI build: $TUI_BUILD | TUI vet: $TUI_VET"
echo "Desktop build: $DESKTOP_BUILD | Desktop vet: $DESKTOP_VET"
