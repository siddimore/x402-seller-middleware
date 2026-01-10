# X402 Seller Middleware for Go

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

## License

MIT
