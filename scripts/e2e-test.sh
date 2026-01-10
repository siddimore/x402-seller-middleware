#!/bin/bash
# End-to-end test script for x402-seller-middleware
# Tests both direct middleware integration and gateway proxy mode

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=========================================="
echo "  x402 Seller Middleware - E2E Tests"
echo "=========================================="
echo ""

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
echo "Building binaries..."
go build -o bin/x402-gateway ./cmd/gateway
go build -o bin/testbackend ./cmd/testbackend
go build -o bin/premium-api ./examples/premium-api
echo -e "${GREEN}✓ Binaries built${NC}"
echo ""

# ==========================================
# Test 1: Gateway Mode (proxy in front of backend)
# ==========================================
echo "=========================================="
echo "Test 1: Gateway Mode"
echo "=========================================="

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
    echo -e "${GREEN}✓ PASS${NC} - Health returned 200"
else
    echo -e "${RED}✗ FAIL${NC} - Expected 200, got $HTTP_CODE"
    exit 1
fi

echo ""
echo "Test 1b: Protected endpoint WITHOUT token"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8402/api/data)
if [ "$HTTP_CODE" = "402" ]; then
    echo -e "${GREEN}✓ PASS${NC} - Protected endpoint returned 402 without token"
else
    echo -e "${RED}✗ FAIL${NC} - Expected 402, got $HTTP_CODE"
    exit 1
fi

echo ""
echo "Test 1c: Protected endpoint WITH valid token"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer valid_test123" http://localhost:8402/api/data)
if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ PASS${NC} - Protected endpoint returned 200 with valid token"
else
    echo -e "${RED}✗ FAIL${NC} - Expected 200, got $HTTP_CODE"
    exit 1
fi

echo ""
echo "Test 1d: Protected endpoint WITH invalid token"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer invalid_token" http://localhost:8402/api/data)
if [ "$HTTP_CODE" = "402" ]; then
    echo -e "${GREEN}✓ PASS${NC} - Protected endpoint returned 402 with invalid token"
else
    echo -e "${RED}✗ FAIL${NC} - Expected 402, got $HTTP_CODE"
    exit 1
fi

echo ""
echo "Test 1e: X-Payment-Token header"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "X-Payment-Token: valid_abc" http://localhost:8402/api/data)
if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ PASS${NC} - X-Payment-Token header works"
else
    echo -e "${RED}✗ FAIL${NC} - Expected 200, got $HTTP_CODE"
    exit 1
fi

echo ""
echo "Test 1f: Query parameter token"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:8402/api/data?payment_token=valid_xyz")
if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ PASS${NC} - Query parameter token works"
else
    echo -e "${RED}✗ FAIL${NC} - Expected 200, got $HTTP_CODE"
    exit 1
fi

# Stop gateway and backend
kill $GATEWAY_PID 2>/dev/null || true
kill $BACKEND_PID 2>/dev/null || true
sleep 1

# ==========================================
# Test 2: Direct Middleware Integration
# ==========================================
echo ""
echo "=========================================="
echo "Test 2: Direct Middleware Integration"
echo "=========================================="

# Start premium API example
echo "Starting premium-api example on :8080..."
./bin/premium-api -port=8080 &
EXAMPLE_PID=$!
sleep 1

echo ""
echo "Test 2a: Free endpoint (article preview list)"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/preview/articles)
if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ PASS${NC} - Free endpoint returned 200"
else
    echo -e "${RED}✗ FAIL${NC} - Expected 200, got $HTTP_CODE"
    exit 1
fi

echo ""
echo "Test 2b: Paid endpoint WITHOUT token"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/articles/1)
if [ "$HTTP_CODE" = "402" ]; then
    echo -e "${GREEN}✓ PASS${NC} - Paid endpoint returned 402 without token"
else
    echo -e "${RED}✗ FAIL${NC} - Expected 402, got $HTTP_CODE"
    exit 1
fi

echo ""
echo "Test 2c: Paid endpoint WITH token"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer valid_premium" http://localhost:8080/api/articles/1)
if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ PASS${NC} - Paid endpoint returned 200 with token"
else
    echo -e "${RED}✗ FAIL${NC} - Expected 200, got $HTTP_CODE"
    exit 1
fi

echo ""
echo "Test 2d: Premium insights (paid)"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "X-Payment-Token: valid_insights" http://localhost:8080/api/premium/insights)
if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ PASS${NC} - Premium insights accessible with token"
else
    echo -e "${RED}✗ FAIL${NC} - Expected 200, got $HTTP_CODE"
    exit 1
fi

# Stop example
kill $EXAMPLE_PID 2>/dev/null || true

echo ""
echo "=========================================="
echo -e "${GREEN}All tests passed! ✓${NC}"
echo "=========================================="
