# X402 ## Features

- 🔐 Intercepts requests and validates payment tokens
- 💰 Returns 402 Payment Required for unauthorized requests
- ⚙️ Configurable payment endpoints and pricing
- 🛣️ Supports exempt paths for public resources
- 🎫 Multiple authentication methods (Bearer, Token, custom headers)
- 🔧 Pluggable payment verifiers (HTTP, Static, JWT, Custom)
- 🚀 Gateway mode for protecting any backend service
- 🌐 Edge-compatible for CDN deployments (Cloudflare, Vercel)
- 📊 **Usage Metering & Analytics** - Track requests, costs, revenue per endpoint
- 🎟️ **Session Payments** - Pay once, use many times (time or request-based)
- 🤖 **AI Agent Optimized** - Budget awareness, batch pricing, auto-retry hintsdleware for Go

A production-ready Go middleware and gateway implementation for HTTP 402 Payment Required protocol.

[![CI](https://github.com/siddimore/x402-seller-middleware/actions/workflows/ci.yml/badge.svg)](https://github.com/siddimore/x402-seller-middleware/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/siddimore/x402-seller-middleware)](https://goreportcard.com/report/github.com/siddimore/x402-seller-middleware)

## Features

- � Intercepts requests and validates payment tokens
- 💰 Returns 402 Payment Required for unauthorized requests
- ⚙️ Configurable payment endpoints and pricing
- 🛣️ Supports exempt paths for public resources
- 🎫 Multiple authentication methods (Bearer, Token, custom headers)
- 🔧 Pluggable payment verifiers (HTTP, Static, JWT, Custom)
- 🚀 Gateway mode for protecting any backend service
- 🌐 Edge-compatible for CDN deployments (Cloudflare, Vercel)

## �📁 Project Structure

```
x402-seller-middleware/
├── cmd/
│   ├── example/          # Basic example server
│   ├── gateway/          # Standalone gateway proxy
│   └── testbackend/      # Test backend for E2E testing
├── pkg/
│   └── x402/             # Public package
│       ├── doc.go        # Package documentation
│       ├── middleware.go # Core middleware
│       ├── middleware_test.go
│       ├── verifier.go   # Payment verification utilities
│       ├── metering.go   # Usage metering & analytics
│       ├── session.go    # Session & subscription payments
│       ├── agent.go      # AI agent optimizations
│       └── edge/         # Edge-compatible handlers
├── examples/
│   └── premium-api/      # Direct middleware integration example
├── scripts/
│   └── e2e-test.sh       # End-to-end test script
├── docs/
│   └── INTEGRATION.md    # Integration guide
├── .github/
│   └── workflows/        # CI/CD pipelines
├── go.mod
├── Makefile
└── README.md
```

## Installation

```bash
go get github.com/siddimore/x402-seller-middleware/pkg/x402
```

## Quick Start

### Option 1: Gateway Mode (Recommended)

Protect any existing backend service without code changes:

```bash
# Build the gateway
make gateway

# Run (protects backend at localhost:3000)
./bin/x402-gateway \
  -backend=http://localhost:3000 \
  -listen=:8402 \
  -payment-url=https://pay.example.com \
  -price=100 \
  -currency=USD \
  -exempt=/health,/public
```

### Option 2: Direct Middleware Integration

```go
package main

import (
    "log"
    "net/http"
    "github.com/siddimore/x402-seller-middleware/pkg/x402"
)

func main() {
    mux := http.NewServeMux()
    
    mux.HandleFunc("/api/protected", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Protected content"))
    })

    handler := x402.Middleware(mux, x402.Config{
        PaymentEndpoint: "https://payment-provider.example.com/verify",
        AcceptedMethods: []string{"Bearer", "Token"},
        PricePerRequest: 100, // cents
        Currency:        "USD",
        ExemptPaths:     []string{"/api/public"},
    })

    log.Fatal(http.ListenAndServe(":8080", handler))
}
```

## Configuration

| Field | Type | Description |
|-------|------|-------------|
| `PaymentEndpoint` | string | URL of your payment verification endpoint |
| `AcceptedMethods` | []string | Supported authentication methods (e.g., "Bearer", "Token") |
| `PricePerRequest` | int64 | Price per request in smallest currency unit |
| `Currency` | string | Currency code (e.g., "USD", "BTC", "ETH") |
| `ExemptPaths` | []string | Path prefixes that don't require payment |
| `PaymentVerifier` | func | Custom payment verification function |

## Payment Verifiers

### HTTP Verifier (Production)

Validate tokens against a remote payment service:

```go
verifier := x402.NewHTTPVerifier(x402.VerifierConfig{
    Endpoint: "https://payment-service.example.com/verify",
    APIKey:   "your-api-key",
    Timeout:  5 * time.Second,
})

handler := x402.Middleware(mux, x402.Config{
    PaymentVerifier: verifier,
    // ... other config
})
```

### Static Verifier (Testing)

Pre-defined list of valid tokens:

```go
verifier := x402.NewStaticVerifier([]string{
    "token_abc123",
    "token_def456",
})

handler := x402.Middleware(mux, x402.Config{
    PaymentVerifier: verifier,
    // ... other config
})
```

### Default Verifier (Development)

Without a custom verifier, tokens starting with `valid_` are accepted:

```bash
curl -H "Authorization: Bearer valid_mytoken" http://localhost:8080/api/resource
```

## Making Requests

### With Valid Payment Token

```bash
# Using Authorization header
curl -H "Authorization: Bearer valid_token123" \
  http://localhost:8080/api/protected

# Using X-Payment-Token header
curl -H "X-Payment-Token: valid_token123" \
  http://localhost:8080/api/protected

# Using query parameter
curl "http://localhost:8080/api/protected?payment_token=valid_token123"
```

### Response for Unpaid Requests (402)

```json
{
  "amount": 100,
  "currency": "USD",
  "payment_url": "https://payment-provider.example.com/verify",
  "description": "Payment of 100 USD required to access this resource"
}
```

**Response headers:**
- `WWW-Authenticate: Bearer, Token realm="Payment Required"`
- `X-Payment-Required: true`
- `X-Payment-Amount: 100`
- `X-Payment-Currency: USD`

### Response for Paid Requests (200)

**Response headers:**
- `X-Payment-Verified: true`
- `X-Payment-Timestamp: 2026-01-10T12:00:00Z`

## Running Tests

```bash
# Unit tests
make test

# E2E tests (gateway + direct middleware)
make e2e-test

# Lint code
make lint
```

## Building

```bash
# Build all binaries
make build

# Build gateway only
make gateway

# Build for all platforms
make build-all
```

## Gateway CLI Reference

```
Usage: x402-gateway [options]

Options:
  -listen string     Listen address (default ":8402", env: LISTEN_ADDR)
  -backend string    Backend URL to proxy to (default "http://localhost:3000", env: BACKEND_URL)
  -payment-url string Payment verification URL (default "https://pay.example.com", env: PAYMENT_URL)
  -price int         Price per request in smallest unit (default 100, env: PRICE_PER_REQUEST)
  -currency string   Currency code (default "USD", env: CURRENCY)
  -exempt string     Comma-separated exempt path prefixes (default "/health", env: EXEMPT_PATHS)
```

## Path Matching

**Important:** Exempt paths use prefix matching. For example:
- `/api/public` exempts `/api/public`, `/api/public/foo`, `/api/publicXYZ`
- `/health` exempts `/health`, `/healthz`, `/health/live`

To avoid unintended matches, use trailing slashes for directories:
```go
ExemptPaths: []string{"/api/public/", "/health"}
```

## Integration Guides

See [docs/INTEGRATION.md](docs/INTEGRATION.md) for detailed integration guides:
- Cloudflare Workers
- Vercel Edge Middleware
- Docker deployment
- Nginx configuration

## USP Features (Differentiated from Coinbase x402)

### 📊 Usage Metering & Analytics

Track API usage in real-time with built-in analytics:

```go
// Create metering store
store := x402.NewInMemoryMeteringStore(100000, "USDC")

// Wrap your handler with metering
handler := x402.MeteringMiddleware(yourHandler, x402.MeteringConfig{
    Store:           store,
    Currency:        "USDC",
    PricePerRequest: 100,
})

// Expose metrics endpoint
http.HandleFunc("/metrics", x402.MetricsHandler(store))
```

**Query metrics:**
```bash
# Get all metrics
curl "http://localhost:8080/metrics"

# Filter by endpoint and AI agents
curl "http://localhost:8080/metrics?endpoint=/api/v1&aiOnly=true"

# Filter by time range
curl "http://localhost:8080/metrics?start=2026-01-01T00:00:00Z&end=2026-01-10T00:00:00Z"
```

**Metrics include:**
- Total requests & revenue
- Requests by hour/day
- Top endpoints by revenue
- Top payers
- AI agent vs human traffic split
- Average latency & error rates

### 🎟️ Session & Subscription Payments

Let users pay once and make multiple requests:

```go
// Create session store
sessionStore := x402.NewInMemorySessionStore()

// Add session middleware
handler := x402.SessionMiddleware(yourHandler, x402.SessionConfig{
    Store:             sessionStore,
    DefaultDuration:   time.Hour,
    DefaultMaxRequests: 100,
    Currency:          "USDC",
})

// Expose session management endpoint
http.HandleFunc("/sessions", x402.SessionHandler(sessionStore, config))
```

**Session types:**
- **Time-based**: Access for a duration (e.g., 1 hour, 24 hours)
- **Request-based**: Fixed number of API calls (e.g., 100 requests)
- **Unlimited**: No limits until expiry

**Create a session:**
```bash
curl -X POST http://localhost:8080/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "payerAddress": "0x123...",
    "paymentProof": "x402_payment_signature",
    "sessionType": "requests",
    "maxRequests": 100
  }'
```

**Use the session:**
```bash
curl -H "X-Session-ID: sess_abc123" http://localhost:8080/api/resource
```

**Response headers include:**
- `X-Session-Remaining: 99 requests`
- `X-Session-Expires: 2026-01-10T15:00:00Z`

### 🤖 AI Agent Optimized Mode

Special handling for AI agents with budget awareness and batch pricing:

```go
handler := x402.AIAgentMiddleware(yourHandler, 
    x402.Config{PricePerRequest: 100, Currency: "USDC"},
    x402.AIAgentConfig{
        EnableBudgetAwareness: true,  // Reject if budget exceeded
        EnableCostEstimation:  true,  // Add cost headers
        EnableAutoRetryHints:  true,  // Add Retry-After on 402
        EnableBatchPricing:    true,  // Discount for batches
        BatchDiscount:         10,    // 10% off for batches
        MinBatchSize:          5,
        Currency:              "USDC",
    },
)

// Cost estimation endpoint
http.HandleFunc("/cost", x402.CostEstimateHandler(pricingMap, "USDC"))

// Agent-friendly welcome endpoint
http.HandleFunc("/", x402.AgentWelcomeHandler(welcomeInfo))
```

**AI Agent Headers (input):**
```bash
curl -H "X-AI-Agent: true" \
     -H "X-Agent-Budget: 10000" \
     -H "X-Agent-Task-ID: task_123" \
     -H "X-Agent-Batch-Size: 10" \
     http://localhost:8080/api/resource
```

**Response headers for AI agents:**
- `X-Estimated-Cost: 100` (before processing)
- `X-Actual-Cost: 100` (after processing)
- `X-Remaining-Budget: 9900`
- `X-Batch-Price-Per-Item: 90` (with batch discount)
- `X-Recommended-Retry: 5` (on 402 responses)
- `Retry-After: 5` (standard HTTP retry header)

**Budget exceeded response:**
```json
{
  "estimatedCost": 1000,
  "costPerRequest": 1000,
  "batchAvailable": true,
  "batchDiscount": 10,
  "retryStrategy": {
    "shouldRetry": false,
    "reason": "Agent budget exceeded"
  },
  "budgetRecommendation": "Increase budget by at least 900 units"
}
```

**Detected AI agents:**
- OpenAI, Anthropic, Claude user agents
- LangChain, AutoGPT, AgentGPT, BabyAGI, CrewAI
- MCP-Client (Model Context Protocol)
- Any request with `X-AI-Agent: true` or `X-Agent-Budget` headers

## License

MIT
