# x402-seller-middleware# X402 ## Features



A production-ready Go middleware for the [x402 HTTP Payment Protocol](https://www.x402.org/). Enables APIs to accept cryptocurrency payments with first-class AI agent support.- 🔐 Intercepts requests and validates payment tokens

- 💰 Returns 402 Payment Required for unauthorized requests

[![CI](https://github.com/siddimore/x402-seller-middleware/actions/workflows/ci.yml/badge.svg)](https://github.com/siddimore/x402-seller-middleware/actions/workflows/ci.yml)- ⚙️ Configurable payment endpoints and pricing

[![Go Report Card](https://goreportcard.com/badge/github.com/siddimore/x402-seller-middleware)](https://goreportcard.com/report/github.com/siddimore/x402-seller-middleware)- 🛣️ Supports exempt paths for public resources

- 🎫 Multiple authentication methods (Bearer, Token, custom headers)

## Features- 🔧 Pluggable payment verifiers (HTTP, Static, JWT, Custom)

- 🚀 Gateway mode for protecting any backend service

- 🔐 **x402 Protocol Compliant** - Full support for X-PAYMENT (v1) and PAYMENT-SIGNATURE (v2) headers- 🌐 Edge-compatible for CDN deployments (Cloudflare, Vercel)

- 💰 **HTTP 402 Responses** - Standards-compliant payment required responses- 📊 **Usage Metering & Analytics** - Track requests, costs, revenue per endpoint

- 🤖 **AI-First Design** - Native MCP server, OpenAI function schemas, pre-authorized budgets- 🎟️ **Session Payments** - Pay once, use many times (time or request-based)

- 📊 **Usage Metering** - Track requests, costs, revenue per endpoint with analytics- 🤖 **AI Agent Optimized** - Budget awareness, batch pricing, auto-retry hintsdleware for Go

- 🎟️ **Session Payments** - Pay once, use many times (time or request-based)

- 🚀 **Gateway Mode** - Protect any backend without code changesA production-ready Go middleware and gateway implementation for HTTP 402 Payment Required protocol.

- 🌐 **Edge Compatible** - Works with Cloudflare Workers, Vercel Edge

[![CI](https://github.com/siddimore/x402-seller-middleware/actions/workflows/ci.yml/badge.svg)](https://github.com/siddimore/x402-seller-middleware/actions/workflows/ci.yml)

## 📁 Project Structure[![Go Report Card](https://goreportcard.com/badge/github.com/siddimore/x402-seller-middleware)](https://goreportcard.com/report/github.com/siddimore/x402-seller-middleware)



```## Features

x402-seller-middleware/

├── pkg/- � Intercepts requests and validates payment tokens

│   ├── x402/                 # Core middleware package- 💰 Returns 402 Payment Required for unauthorized requests

│   │   ├── middleware.go     # HTTP 402 middleware- ⚙️ Configurable payment endpoints and pricing

│   │   ├── verifier.go       # Payment verification- 🛣️ Supports exempt paths for public resources

│   │   ├── metering.go       # Usage analytics- 🎫 Multiple authentication methods (Bearer, Token, custom headers)

│   │   ├── session.go        # Session payments- 🔧 Pluggable payment verifiers (HTTP, Static, JWT, Custom)

│   │   ├── agent.go          # AI agent detection & pricing- 🚀 Gateway mode for protecting any backend service

│   │   ├── ai_http.go        # AI HTTP integration (discovery, budgets)- 🌐 Edge-compatible for CDN deployments (Cloudflare, Vercel)

│   │   └── edge/             # Edge runtime handlers

│   └── mcp/                  # MCP Server for AI agents## �📁 Project Structure

│       └── server.go         # Model Context Protocol server

├── cmd/```

│   ├── gateway/              # Standalone gateway proxyx402-seller-middleware/

│   ├── x402-mcp/             # MCP server CLI├── cmd/

│   └── example/              # Basic example│   ├── example/          # Basic example server

├── examples/│   ├── gateway/          # Standalone gateway proxy

│   └── premium-api/          # Full integration example│   └── testbackend/      # Test backend for E2E testing

└── scripts/├── pkg/

    └── e2e-test.sh           # End-to-end tests│   └── x402/             # Public package

```│       ├── doc.go        # Package documentation

│       ├── middleware.go # Core middleware

## Installation│       ├── middleware_test.go

│       ├── verifier.go   # Payment verification utilities

```bash│       ├── metering.go   # Usage metering & analytics

go get github.com/siddimore/x402-seller-middleware/pkg/x402│       ├── session.go    # Session & subscription payments

```│       ├── agent.go      # AI agent optimizations

│       └── edge/         # Edge-compatible handlers

## Quick Start├── examples/

│   └── premium-api/      # Direct middleware integration example

### Option 1: Gateway Mode (Zero Code Changes)├── scripts/

│   └── e2e-test.sh       # End-to-end test script

Protect any existing backend:├── docs/

│   └── INTEGRATION.md    # Integration guide

```bash├── .github/

# Build│   └── workflows/        # CI/CD pipelines

make gateway├── go.mod

├── Makefile

# Run└── README.md

./bin/x402-gateway \```

  -backend=http://localhost:3000 \

  -listen=:8402 \## Installation

  -price=100 \

  -currency=USDC \```bash

  -network=base \go get github.com/siddimore/x402-seller-middleware/pkg/x402

  -payto=0xYourWallet \```

  -exempt=/health,/public

```## Quick Start



### Option 2: Direct Middleware### Option 1: Gateway Mode (Recommended)



```goProtect any existing backend service without code changes:

package main

```bash

import (# Build the gateway

    "net/http"make gateway

    "github.com/siddimore/x402-seller-middleware/pkg/x402"

)# Run (protects backend at localhost:3000)

./bin/x402-gateway \

func main() {  -backend=http://localhost:3000 \

    mux := http.NewServeMux()  -listen=:8402 \

    mux.HandleFunc("/api/data", dataHandler)  -payment-url=https://pay.example.com \

  -price=100 \

    handler := x402.Middleware(mux, x402.Config{  -currency=USD \

        PayTo:           "0xYourWalletAddress",  -exempt=/health,/public

        Network:         "base",```

        Currency:        "USDC",

        PricePerRequest: 100,  // 0.0001 USDC (6 decimals)### Option 2: Direct Middleware Integration

        ExemptPaths:     []string{"/health", "/public/"},

    })```go

package main

    http.ListenAndServe(":8080", handler)

}import (

```    "log"

    "net/http"

## x402 Protocol Support    "github.com/siddimore/x402-seller-middleware/pkg/x402"

)

### Payment Headers

func main() {

The middleware accepts payments via:    mux := http.NewServeMux()

    

| Header | Version | Example |    mux.HandleFunc("/api/protected", func(w http.ResponseWriter, r *http.Request) {

|--------|---------|---------|        w.Write([]byte("Protected content"))

| `X-PAYMENT` | v1 | `X-PAYMENT: <base64-encoded-payment>` |    })

| `PAYMENT-SIGNATURE` | v2 | `PAYMENT-SIGNATURE: <signature>` |

| `Authorization` | Bearer | `Authorization: Bearer <token>` |    handler := x402.Middleware(mux, x402.Config{

| `X-Payment-Token` | Custom | `X-Payment-Token: <token>` |        PaymentEndpoint: "https://payment-provider.example.com/verify",

| Query param | - | `?payment_token=<token>` |        AcceptedMethods: []string{"Bearer", "Token"},

        PricePerRequest: 100, // cents

### 402 Response Format        Currency:        "USD",

        ExemptPaths:     []string{"/api/public"},

```json    })

{

  "x402Version": 1,    log.Fatal(http.ListenAndServe(":8080", handler))

  "accepts": [{}

    "scheme": "exact",```

    "network": "base",

    "maxAmountRequired": "100",## Configuration

    "resource": "/api/data",

    "description": "Access to premium API",| Field | Type | Description |

    "mimeType": "application/json",|-------|------|-------------|

    "payTo": "0x...",| `PaymentEndpoint` | string | URL of your payment verification endpoint |

    "maxTimeoutSeconds": 300,| `AcceptedMethods` | []string | Supported authentication methods (e.g., "Bearer", "Token") |

    "asset": "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"| `PricePerRequest` | int64 | Price per request in smallest currency unit |

  }],| `Currency` | string | Currency code (e.g., "USD", "BTC", "ETH") |

  "error": "Payment required"| `ExemptPaths` | []string | Path prefixes that don't require payment |

}| `PaymentVerifier` | func | Custom payment verification function |

```

## Payment Verifiers

### Response Headers

### HTTP Verifier (Production)

**On 402 (Payment Required):**

- `X-Payment-Required: true`Validate tokens against a remote payment service:

- `X-Payment-Amount: 100`

- `X-Payment-Currency: USDC````go

- `X-Payment-Network: base`verifier := x402.NewHTTPVerifier(x402.VerifierConfig{

    Endpoint: "https://payment-service.example.com/verify",

**On 200 (Payment Verified):**    APIKey:   "your-api-key",

- `X-Payment-Verified: true`    Timeout:  5 * time.Second,

- `X-Payment-Timestamp: 2026-01-10T12:00:00Z`})



## AI Agent Supporthandler := x402.Middleware(mux, x402.Config{

    PaymentVerifier: verifier,

### MCP Server (for Claude, GPT, etc.)    // ... other config

})

Run the MCP server for AI agents to automatically discover and pay for APIs:```



```bash### Static Verifier (Testing)

# Build MCP server

go build -o bin/x402-mcp ./cmd/x402-mcpPre-defined list of valid tokens:



# Run with stdio (for Claude Desktop)```go

./bin/x402-mcp --stdio --network=base --currency=USDC --budget=10000verifier := x402.NewStaticVerifier([]string{

    "token_abc123",

# Or with HTTP transport    "token_def456",

./bin/x402-mcp --http=:9402 --network=base --currency=USDC})

```

handler := x402.Middleware(mux, x402.Config{

**Claude Desktop configuration** (`~/.claude/claude_desktop_config.json`):    PaymentVerifier: verifier,

```json    // ... other config

{})

  "mcpServers": {```

    "x402": {

      "command": "/path/to/x402-mcp",### Default Verifier (Development)

      "args": ["--stdio", "--network=base", "--currency=USDC"]

    }Without a custom verifier, tokens starting with `valid_` are accepted:

  }

}```bash

```curl -H "Authorization: Bearer valid_mytoken" http://localhost:8080/api/resource

```

**MCP Tools available:**

- `x402_discover` - Discover API capabilities via 402 response## Making Requests

- `x402_budget` - Create/manage payment budgets

- `x402_call` - Make paid API calls### With Valid Payment Token

- `x402_estimate` - Estimate cost before calling

- `x402_history` - View call history```bash

# Using Authorization header

### AI HTTP Integrationcurl -H "Authorization: Bearer valid_token123" \

  http://localhost:8080/api/protected

For API servers, add AI-friendly endpoints:

# Using X-Payment-Token header

```gocurl -H "X-Payment-Token: valid_token123" \

import "github.com/siddimore/x402-seller-middleware/pkg/x402"  http://localhost:8080/api/protected



// AI discovery endpoint - returns OpenAI/MCP schemas# Using query parameter

http.HandleFunc("/ai/discover", x402.AIDiscoveryHandler(config))curl "http://localhost:8080/api/protected?payment_token=valid_token123"

```

// Pre-authorized budgets endpoint

http.HandleFunc("/ai/budget", x402.AIBudgetHandler(preAuthStore, config))### Response for Unpaid Requests (402)



// Wrap handlers with AI-first middleware```json

handler := x402.AIFirstMiddleware(yourHandler, x402.AIFirstConfig{{

    EnablePreAuth:    true,  "amount": 100,

    EnableIdempotency: true,  "currency": "USD",

    PreAuthStore:     x402.NewInMemoryPreAuthStore(),  "payment_url": "https://payment-provider.example.com/verify",

    IdempotencyStore: x402.NewInMemoryIdempotencyStore(),  "description": "Payment of 100 USD required to access this resource"

    Endpoints:        apiEndpoints,}

})```

```

**Response headers:**

**AI Agent Headers (input):**- `WWW-Authenticate: Bearer, Token realm="Payment Required"`

```bash- `X-Payment-Required: true`

curl -H "X-AI-Agent: true" \- `X-Payment-Amount: 100`

     -H "X-Agent-ID: my-agent" \- `X-Payment-Currency: USD`

     -H "X-Agent-Budget: 10000" \

     -H "X-Idempotency-Key: unique-request-id" \### Response for Paid Requests (200)

     http://localhost:8080/api/data

```**Response headers:**

- `X-Payment-Verified: true`

**Response headers for AI:**- `X-Payment-Timestamp: 2026-01-10T12:00:00Z`

- `X-Request-ID: req_abc123`

- `X-Budget-Remaining: 9900`## Running Tests

- `X-Budget-Deducted: 100`

```bash

## Usage Metering# Unit tests

make test

Track API usage with built-in analytics:

# E2E tests (gateway + direct middleware)

```gomake e2e-test

store := x402.NewInMemoryMeteringStore(100000, "USDC")

# Lint code

handler := x402.MeteringMiddleware(yourHandler, x402.MeteringConfig{make lint

    Store:           store,```

    Currency:        "USDC",

    PricePerRequest: 100,## Building

})

```bash

// Metrics endpoint# Build all binaries

http.HandleFunc("/metrics", x402.MetricsHandler(store))make build

```

# Build gateway only

**Query metrics:**make gateway

```bash

curl "http://localhost:8080/metrics?endpoint=/api/v1&aiOnly=true"# Build for all platforms

```make build-all

```

**Metrics include:**

- Total requests & revenue## Gateway CLI Reference

- Requests by endpoint

- AI vs human traffic split```

- Top payersUsage: x402-gateway [options]

- Average latency

Options:

## Session Payments  -listen string     Listen address (default ":8402", env: LISTEN_ADDR)

  -backend string    Backend URL to proxy to (default "http://localhost:3000", env: BACKEND_URL)

Let users pay once for multiple requests:  -payment-url string Payment verification URL (default "https://pay.example.com", env: PAYMENT_URL)

  -price int         Price per request in smallest unit (default 100, env: PRICE_PER_REQUEST)

```go  -currency string   Currency code (default "USD", env: CURRENCY)

sessionStore := x402.NewInMemorySessionStore()  -exempt string     Comma-separated exempt path prefixes (default "/health", env: EXEMPT_PATHS)

```

handler := x402.SessionMiddleware(yourHandler, x402.SessionConfig{

    Store:              sessionStore,## Path Matching

    DefaultDuration:    time.Hour,

    DefaultMaxRequests: 100,**Important:** Exempt paths use prefix matching. For example:

})- `/api/public` exempts `/api/public`, `/api/public/foo`, `/api/publicXYZ`

- `/health` exempts `/health`, `/healthz`, `/health/live`

http.HandleFunc("/sessions", x402.SessionHandler(sessionStore, config))

```To avoid unintended matches, use trailing slashes for directories:

```go

**Create session:**ExemptPaths: []string{"/api/public/", "/health"}

```bash```

curl -X POST http://localhost:8080/sessions \

  -d '{"payerAddress":"0x...","sessionType":"requests","maxRequests":100}'## Integration Guides

```

See [docs/INTEGRATION.md](docs/INTEGRATION.md) for detailed integration guides:

**Use session:**- Cloudflare Workers

```bash- Vercel Edge Middleware

curl -H "X-Session-ID: sess_abc123" http://localhost:8080/api/data- Docker deployment

```- Nginx configuration



## Running Tests## USP Features (Differentiated from Coinbase x402)



```bash### 📊 Usage Metering & Analytics

# Unit tests

make testTrack API usage in real-time with built-in analytics:



# E2E tests```go

make e2e-test// Create metering store

store := x402.NewInMemoryMeteringStore(100000, "USDC")

# Lint

make lint// Wrap your handler with metering

```handler := x402.MeteringMiddleware(yourHandler, x402.MeteringConfig{

    Store:           store,

## Building    Currency:        "USDC",

    PricePerRequest: 100,

```bash})

# All binaries

make build// Expose metrics endpoint

http.HandleFunc("/metrics", x402.MetricsHandler(store))

# Gateway only```

make gateway

**Query metrics:**

# MCP server only```bash

go build -o bin/x402-mcp ./cmd/x402-mcp# Get all metrics

```curl "http://localhost:8080/metrics"



## Configuration Reference# Filter by endpoint and AI agents

curl "http://localhost:8080/metrics?endpoint=/api/v1&aiOnly=true"

### Middleware Config

# Filter by time range

| Field | Type | Description |curl "http://localhost:8080/metrics?start=2026-01-01T00:00:00Z&end=2026-01-10T00:00:00Z"

|-------|------|-------------|```

| `PayTo` | string | Wallet address to receive payments |

| `Network` | string | Blockchain network (base, base-sepolia) |**Metrics include:**

| `Currency` | string | Currency code (USDC) |- Total requests & revenue

| `PricePerRequest` | int64 | Price in smallest unit |- Requests by hour/day

| `ExemptPaths` | []string | Paths that don't require payment |- Top endpoints by revenue

| `PaymentVerifier` | func | Custom verification function |- Top payers

- AI agent vs human traffic split

### Gateway CLI- Average latency & error rates



```### 🎟️ Session & Subscription Payments

./bin/x402-gateway [options]

Let users pay once and make multiple requests:

  -listen string      Listen address (default ":8402")

  -backend string     Backend URL to proxy```go

  -price int          Price per request (default 100)// Create session store

  -currency string    Currency (default "USDC")sessionStore := x402.NewInMemorySessionStore()

  -network string     Network (default "base")

  -payto string       Wallet address// Add session middleware

  -exempt string      Comma-separated exempt pathshandler := x402.SessionMiddleware(yourHandler, x402.SessionConfig{

```    Store:             sessionStore,

    DefaultDuration:   time.Hour,

### MCP Server CLI    DefaultMaxRequests: 100,

    Currency:          "USDC",

```})

./bin/x402-mcp [options]

// Expose session management endpoint

  --stdio             Use stdio transport (for Claude Desktop)http.HandleFunc("/sessions", x402.SessionHandler(sessionStore, config))

  --http string       HTTP listen address (e.g., ":9402")```

  --network string    Blockchain network (default "base")

  --currency string   Currency (default "USDC")**Session types:**

  --budget int        Default budget in smallest unit- **Time-based**: Access for a duration (e.g., 1 hour, 24 hours)

```- **Request-based**: Fixed number of API calls (e.g., 100 requests)

- **Unlimited**: No limits until expiry

## Integration Examples

**Create a session:**

See [examples/premium-api](examples/premium-api) for a complete integration example.```bash

curl -X POST http://localhost:8080/sessions \

## License  -H "Content-Type: application/json" \

  -d '{

MIT    "payerAddress": "0x123...",

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
