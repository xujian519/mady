#!/bin/bash
set +e
ROOT=/Users/xujian/projects/Mady
cd "$ROOT"

echo "=========================================="
echo " 1. ROOT MODULE: go build ./..."
echo "=========================================="
cd "$ROOT" && go build ./... 2>&1
RC1=$?
echo "Exit code: $RC1"
echo ""

echo "=========================================="
echo " 2. TUI MODULE: go build ./..."
echo "=========================================="
cd "$ROOT/tui" && go build ./... 2>&1
RC2=$?
echo "Exit code: $RC2"
echo ""

echo "=========================================="
echo " 3. TOOLS MODULE: go build ./..."
echo "=========================================="
cd "$ROOT/tools" && go build ./... 2>&1
RC3=$?
echo "Exit code: $RC3"
echo ""

echo "=========================================="
echo " 4. DESKTOP MODULE: go build ./..."
echo "=========================================="
cd "$ROOT/desktop" && go build ./... 2>&1
RC4=$?
echo "Exit code: $RC4"
echo ""

echo "=========================================="
echo " 5. ROOT MODULE: go vet ./..."
echo "=========================================="
cd "$ROOT" && go vet ./... 2>&1
RC5=$?
echo "Exit code: $RC5"
echo ""

echo "=========================================="
echo " 6. TUI MODULE: go vet ./..."
echo "=========================================="
cd "$ROOT/tui" && go vet ./... 2>&1
RC6=$?
echo "Exit code: $RC6"
echo ""

echo "=========================================="
echo " 7. TOOLS MODULE: go vet ./..."
echo "=========================================="
cd "$ROOT/tools" && go vet ./... 2>&1
RC7=$?
echo "Exit code: $RC7"
echo ""

echo "=========================================="
echo " 8. DESKTOP MODULE: go vet ./..."
echo "=========================================="
cd "$ROOT/desktop" && go vet ./... 2>&1
RC8=$?
echo "Exit code: $RC8"
echo ""

echo "=========================================="
echo " SUMMARY"
echo "=========================================="
echo "Module        | Build | Vet"
echo "--------------|-------|-----"
echo "Root          | $RC1     | $RC5"
echo "Tools         | $RC3     | $RC7"
echo "TUI           | $RC2     | $RC6"
echo "Desktop       | $RC4     | $RC8"

echo ""
if [ $RC1 -eq 0 ] && [ $RC2 -eq 0 ] && [ $RC3 -eq 0 ] && [ $RC4 -eq 0 ] && [ $RC5 -eq 0 ] && [ $RC6 -eq 0 ] && [ $RC7 -eq 0 ] && [ $RC8 -eq 0 ]; then
    echo "🎉 ALL CHECKS PASSED"
else
    echo "❌ SOME CHECKS FAILED - see above for details"
fi

exit $(($RC1+$RC2+$RC3+$RC4+$RC5+$RC6+$RC7+$RC8))
