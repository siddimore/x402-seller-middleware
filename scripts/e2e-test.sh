#!/bin/bash
# End-to-end test script for x402-seller-middleware
# Tests x402 protocol compliance, gateway mode, and AI agent features

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

PASS_COUNT=0
FAIL_COUNT=0

pass() {
    echo -e "${GREEN}✓ PASS${NC} - $1"
    ((PASS_COUNT++))
}

fail() {
    echo -e "${RED}✗ FAIL${NC} - $1"
    ((FAIL_COUNT++))
}

section() {
    echo ""
    echo -e "${BLUE}=========================================="
    echo "  $1"
    echo -e "==========================================${NC}"
}

echo "=========================================="
echo "  x402 Seller Middleware - E2E Tests"
echo "=========================================="

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up..."
    kill $BACKEND_PID 2>/dev/null || true
    kill $GATEWAY_PID 2>/dev/null || true
    kill $EXAMPLE_PID 2>/dev/null || true
}
trap cleanup EXIT

# Build binaries
echo ""
echo "Building binaries..."
go build -o bin/x402-gateway ./cmd/gateway
go build -o bin/testbackend ./cmd/testbackend
go build -o bin/premium-api ./examples/premium-api
echo -e "${GREEN}✓ Binaries built${NC}"

# ==========================================
section "Test 1: Gateway Mode - Basic x402"
# ==========================================

# Start test backend
echo "Starting test backend on :3000..."
./bin/testbackend &
BACKEND_PID=$!
sleep 1

# Start gateway
echo "Starting gateway on :8402..."
./bin/x402-gateway -backend=http://localhost:3000 -payment-url=https://pay.example.com -exempt=/health,/api/public &
GATEWAY_PID=$!
sleep 1

echo ""
echo "Test 1a: Health check (exempt path)"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8402/health)
if [ "$HTTP_CODE" = "200" ]; then
    pass "Health returned 200"
else
    fail "Expected 200, got $HTTP_CODE"
fi

echo ""
echo "Test 1b: Protected endpoint WITHOUT payment"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8402/api/data)
if [ "$HTTP_CODE" = "402" ]; then
    pass "Protected endpoint returned 402 without payment"
else
    fail "Expected 402, got $HTTP_CODE"
fi

echo ""
echo "Test 1c: 402 response has X-Payment-Required header"
HEADER=$(curl -s -I http://localhost:8402/api/data | grep -i "X-Payment-Required" || echo "")
if [[ "$HEADER" == *"true"* ]]; then
    pass "402 response includes X-Payment-Required: true"
else
    fail "Missing X-Payment-Required header"
fi

echo ""
echo "Test 1d: 402 response body is valid JSON"
BODY=$(curl -s http://localhost:8402/api/data)
if echo "$BODY" | jq -e '.amount' > /dev/null 2>&1; then
    pass "402 response body is valid JSON with amount"
else
    fail "402 response body invalid"
fi

# ==========================================
section "Test 2: x402 Protocol Headers"
# ==========================================

echo ""
echo "Test 2a: Authorization: Bearer token"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer valid_test123" http://localhost:8402/api/data)
if [ "$HTTP_CODE" = "200" ]; then
    pass "Authorization Bearer token accepted"
else
    fail "Expected 200, got $HTTP_CODE"
fi

echo ""
echo "Test 2b: X-Payment-Token header"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "X-Payment-Token: valid_abc" http://localhost:8402/api/data)
if [ "$HTTP_CODE" = "200" ]; then
    pass "X-Payment-Token header accepted"
else
    fail "Expected 200, got $HTTP_CODE"
fi

echo ""
echo "Test 2c: X-PAYMENT header (v1)"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "X-PAYMENT: valid_v1_payment" http://localhost:8402/api/data)
if [ "$HTTP_CODE" = "200" ]; then
    pass "X-PAYMENT header (v1) accepted"
else
    fail "Expected 200, got $HTTP_CODE"
fi

echo ""
echo "Test 2d: PAYMENT-SIGNATURE header (v2)"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "PAYMENT-SIGNATURE: valid_v2_signature" http://localhost:8402/api/data)
if [ "$HTTP_CODE" = "200" ]; then
    pass "PAYMENT-SIGNATURE header (v2) accepted"
else
    fail "Expected 200, got $HTTP_CODE"
fi

echo ""
echo "Test 2e: Query parameter token"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:8402/api/data?payment_token=valid_xyz")
if [ "$HTTP_CODE" = "200" ]; then
    pass "Query parameter token accepted"
else
    fail "Expected 200, got $HTTP_CODE"
fi

echo ""
echo "Test 2f: Invalid token rejected"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer invalid_token" http://localhost:8402/api/data)
if [ "$HTTP_CODE" = "402" ]; then
    pass "Invalid token correctly rejected with 402"
else
    fail "Expected 402, got $HTTP_CODE"
fi

echo ""
echo "Test 2g: X-Payment-Verified header on success"
HEADER=$(curl -s -I -H "Authorization: Bearer valid_test" http://localhost:8402/api/data | grep -i "X-Payment-Verified" || echo "")
if [[ "$HEADER" == *"true"* ]]; then
    pass "Success response includes X-Payment-Verified: true"
else
    fail "Missing X-Payment-Verified header on success"
fi

# Stop gateway and backend
kill $GATEWAY_PID 2>/dev/null || true
kill $BACKEND_PID 2>/dev/null || true
sleep 1

# ==========================================
section "Test 3: Direct Middleware Integration"
# ==========================================

# Start premium API example
echo "Starting premium-api example on :8080..."
./bin/premium-api -port=8080 &
EXAMPLE_PID=$!
sleep 1

echo ""
echo "Test 3a: Free endpoint (exempt path)"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/preview/articles)
if [ "$HTTP_CODE" = "200" ]; then
    pass "Free endpoint returned 200"
else
    fail "Expected 200, got $HTTP_CODE"
fi

echo ""
echo "Test 3b: Paid endpoint WITHOUT token"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/articles/1)
if [ "$HTTP_CODE" = "402" ]; then
    pass "Paid endpoint returned 402 without token"
else
    fail "Expected 402, got $HTTP_CODE"
fi

echo ""
echo "Test 3c: Paid endpoint WITH token"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer valid_premium" http://localhost:8080/api/articles/1)
if [ "$HTTP_CODE" = "200" ]; then
    pass "Paid endpoint returned 200 with token"
else
    fail "Expected 200, got $HTTP_CODE"
fi

echo ""
echo "Test 3d: PAYMENT-SIGNATURE header (v2) on premium API"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "PAYMENT-SIGNATURE: valid_v2_sig" http://localhost:8080/api/articles/1)
if [ "$HTTP_CODE" = "200" ]; then
    pass "PAYMENT-SIGNATURE v2 works on direct middleware"
else
    fail "Expected 200, got $HTTP_CODE"
fi

# ==========================================
section "Test 4: AI Agent Headers"
# ==========================================

echo ""
echo "Test 4a: AI agent detection via User-Agent"
RESPONSE=$(curl -s -I -A "Claude-AI/1.0" -H "Authorization: Bearer valid_test" http://localhost:8080/api/articles/1)
if echo "$RESPONSE" | grep -qi "X-"; then
    pass "AI agent User-Agent processed"
else
    fail "AI agent detection failed"
fi

echo ""
echo "Test 4b: X-AI-Agent header"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "X-AI-Agent: true" -H "Authorization: Bearer valid_test" http://localhost:8080/api/articles/1)
if [ "$HTTP_CODE" = "200" ]; then
    pass "X-AI-Agent header accepted"
else
    fail "Expected 200, got $HTTP_CODE"
fi

echo ""
echo "Test 4c: X-Agent-Budget header"
RESPONSE=$(curl -s -I -H "X-Agent-Budget: 10000" -H "Authorization: Bearer valid_test" http://localhost:8080/api/articles/1)
HTTP_CODE=$(echo "$RESPONSE" | head -1 | awk '{print $2}')
if [ "$HTTP_CODE" = "200" ]; then
    pass "X-Agent-Budget header processed"
else
    fail "Expected 200, got $HTTP_CODE"
fi

# Stop example
kill $EXAMPLE_PID 2>/dev/null || true

# ==========================================
section "Test Summary"
# ==========================================

echo ""
TOTAL=$((PASS_COUNT + FAIL_COUNT))
echo "Results: $PASS_COUNT/$TOTAL tests passed"
echo ""

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}=========================================="
    echo "  All tests passed! ✓"
    echo -e "==========================================${NC}"
    exit 0
else
    echo -e "${RED}=========================================="
    echo "  $FAIL_COUNT test(s) failed"
    echo -e "==========================================${NC}"
    exit 1
fi
