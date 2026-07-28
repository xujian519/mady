#!/bin/bash
set -e
echo "=== 1. Root module: go build ./... ==="
cd /Users/xujian/projects/Mady && go build ./... 2>&1
echo "SUCCESS"
echo ""
echo "=== 2. TUI sub-module: go build ./... ==="
cd /Users/xujian/projects/Mady/tui && go build ./... 2>&1
echo "SUCCESS"
echo ""
echo "=== 3. Tools sub-module: go build ./... ==="
cd /Users/xujian/projects/Mady/tools && go build ./... 2>&1
echo "SUCCESS"
echo ""
echo "=== 4. Desktop sub-module: go build ./... ==="
cd /Users/xujian/projects/Mady/desktop && go build ./... 2>&1
echo "SUCCESS"
echo ""
echo "=== 5. Root module: go vet ./... ==="
cd /Users/xujian/projects/Mady && go vet ./... 2>&1
echo "SUCCESS"
echo ""
echo "=== 6. TUI sub-module: go vet ./... ==="
cd /Users/xujian/projects/Mady/tui && go vet ./... 2>&1
echo "SUCCESS"
echo ""
echo "=== 7. Tools sub-module: go vet ./... ==="
cd /Users/xujian/projects/Mady/tools && go vet ./... 2>&1
echo "SUCCESS"
echo ""
echo "=== 8. Desktop sub-module: go vet ./... ==="
cd /Users/xujian/projects/Mady/desktop && go vet ./... 2>&1
echo "SUCCESS"
