#!/bin/bash
# Test runner for the proxy/sanitizer — runs Go tests for the proxy package

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

RED=$'\033[0;31m'
GREEN=$'\033[0;32m'
NC=$'\033[0m'

echo "═══════════════════════════════════════════════════════════════"
echo "  Alcatraz Proxy Sanitizer — Test Suite (Go)"
echo "═══════════════════════════════════════════════════════════════"
echo ""

GO_BIN="$(which go 2>/dev/null || echo /usr/local/go/bin/go)"

if [ ! -x "$GO_BIN" ]; then
    echo "[ERROR] go not found. Install Go to run the tests."
    exit 1
fi

set +e
(cd "$SCRIPT_DIR/platform/backend" && "$GO_BIN" test ./internal/proxy/... -v) 2>&1
EXIT_CODE=$?
set -e

echo ""
if [ $EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}[✓] All proxy/sanitizer tests passed!${NC}"
    echo ""
    echo "  The sanitizer is correctly:"
    echo "    • Detecting secrets (API keys, PII, cloud credentials)"
    echo "    • Sanitizing JSON payloads without corrupting structure"
    echo "    • Replacing real values with placeholders [REDACTED_*]"
    echo "    - Not producing false positives on innocent text"
    echo ""
else
    echo -e "${RED}[✗] Some tests FAILED!${NC}"
    exit $EXIT_CODE
fi
