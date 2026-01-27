# x402-seller-middleware

A production-ready Go middleware for the [x402 HTTP Payment Protocol](https://www.x402.org/). Add cryptocurrency payment requirements to any HTTP API with minimal code changes.

[![CI](https://github.com/siddimore/x402-seller-middleware/actions/workflows/ci.yml/badge.svg)](https://github.com/siddimore/x402-seller-middleware/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/siddimore/x402-seller-middleware)](https://goreportcard.com/report/github.com/siddimore/x402-seller-middleware)
[![Go Reference](https://pkg.go.dev/badge/github.com/siddimore/x402-seller-middleware.svg)](https://pkg.go.dev/github.com/siddimore/x402-seller-middleware)

## 🎯 What is x402?

x402 is an open payment protocol that uses HTTP status code 402 (Payment Required) to enable:
- **Pay-per-request APIs** - Charge users per API call with USDC
- **AI Agent Payments** - Let autonomous agents pay for resources
- **No API Keys** - Replace subscriptions with direct payments

## ✨ Features

- 🔐 **x402 Protocol Compliant** - Full support for v1 (`X-PAYMENT`) header format
- 💰 **USDC Payments** - EIP-3009 `transferWithAuthorization` on Base/Base Sepolia
- ⚡ **Facilitator Integration** - Automatic verification via Coinbase facilitator
- 🚀 **Gateway Mode** - Protect any backend without code changes
- 🤖 **AI-Optimized** - Budget hints, batch pricing, agent detection
- 📊 **Usage Metering** - Track requests, costs, and revenue
- 🎟️ **Session Payments** - Pay once, use many times
- 🌐 **Edge Compatible** - Works with Cloudflare Workers, Vercel Edge

## 📦 Installation

```bash
go get github.com/siddimore/x402-seller-middleware/pkg/x402
```

## 🚀 Quick Start

### Option 1: Gateway Mode (Zero Code Changes)

Protect any existing backend:

```bash
# Build
make gateway

# Run
./bin/x402-gateway \
  -backend=http://localhost:3000 \
  -listen=:8402 \
  -price=100 \
  -currency=USDC \
  -network=base-sepolia \
  -payto=0xYourWallet \
  -facilitator=https://x402.org/facilitator \
  -exempt=/health,/public
```

### Option 2: Direct Middleware

```go
package main

import (
    "net/http"
    "github.com/siddimore/x402-seller-middleware/pkg/x402"
)

func main() {
    mux := http.NewServeMux()
    mux.HandleFunc("/api/data", dataHandler)

    // Wrap with x402 middleware
    handler := x402.Middleware(mux, x402.Config{
        PayTo:           "0xYourWalletAddress",
        Network:         "base-sepolia",
        Currency:        "USDC",
        Asset:           "0x036CbD53842c5426634e7929541eC2318f3dCF7e", // USDC on Base Sepolia
        PricePerRequest: 100,  // 100 micro-USDC = $0.0001
        FacilitatorURL:  "https://x402.org/facilitator",
        ExemptPaths:     []string{"/health", "/public/"},
    })

    http.ListenAndServe(":8080", handler)
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte(`{"data": "premium content"}`))
}
```

## 🔄 Payment Flow

```
┌──────────┐     GET /api/data      ┌──────────────┐
│  Client  │ ─────────────────────▶ │   x402       │
│          │                        │  Middleware  │
│          │ ◀───────────────────── │              │
│          │   402 Payment Required │              │
│          │   {accepts: [...]}     │              │
│          │                        └──────────────┘
│          │                               │
│          │  Sign payment with wallet     │
│          │                               │
│          │  GET /api/data                │
│          │  X-PAYMENT: <signed>   ┌──────────────┐
│          │ ─────────────────────▶ │   x402       │
│          │                        │  Middleware  │
│          │                        │      │       │
│          │                        │      ▼       │
│          │                        │ ┌──────────┐ │
│          │                        │ │Facilitator│ │
│          │                        │ │ Verify   │ │
│          │                        │ └──────────┘ │
│          │ ◀───────────────────── │              │
└──────────┘   200 OK + data        └──────────────┘
```

## 📋 402 Response Format

When a request lacks payment, the middleware returns:

```json
{
  "error": "Payment required",
  "accepts": [
    {
      "scheme": "exact",
      "network": "base-sepolia",
      "maxAmountRequired": "100",
      "resource": "/api/data",
      "description": "Access to premium data endpoint",
      "mimeType": "application/json",
      "payTo": "0xYourWalletAddress",
      "maxTimeoutSeconds": 30,
      "asset": "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
      "extra": {
        "name": "USDC",
        "version": "2"
      }
    }
  ]
}
```

## 🔑 EIP-712 Signing

The middleware uses EIP-3009 `TransferWithAuthorization` for gasless USDC transfers:

```go
// Domain for Base Sepolia USDC
domain := apitypes.TypedDataDomain{
    Name:              "USDC",
    Version:           "2",
    ChainId:           math.NewHexOrDecimal256(84532),
    VerifyingContract: "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
}

// Message structure
message := map[string]interface{}{
    "from":        payerAddress,
    "to":          payToAddress,
    "value":       amount,
    "validAfter":  "0",
    "validBefore": deadline,
    "nonce":       randomNonce,
}
```

## 📁 Project Structure

```
x402-seller-middleware/
├── pkg/
│   └── x402/                 # Public package
│       ├── middleware.go     # Core HTTP middleware
│       ├── unified_middleware.go  # Unified payment handling
│       ├── payment_rails.go  # EVM crypto payment rail
│       ├── verifier.go       # Payment verification
│       ├── metering.go       # Usage analytics
│       ├── session.go        # Session payments
│       ├── agent.go          # AI agent detection
│       └── edge/             # Edge runtime handlers
├── cmd/
│   ├── gateway/              # Standalone gateway
│   ├── example/              # Basic example
│   └── testbackend/          # Test backend
├── examples/
│   └── premium-api/          # Full integration example
├── deploy/
│   ├── cloudflare-worker/    # Cloudflare Workers deployment
│   ├── vercel-edge/          # Vercel Edge deployment
│   └── docker/               # Docker deployment
└── scripts/
    └── e2e-test.sh           # End-to-end tests
```

## 🌐 Supported Networks

| Network | Chain ID | USDC Contract | EIP-712 Domain |
|---------|----------|---------------|----------------|
| Base Sepolia | 84532 | `0x036CbD53842c5426634e7929541eC2318f3dCF7e` | name: "USDC", version: "2" |
| Base Mainnet | 8453 | `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` | name: "USDC", version: "2" |

## ⚙️ Configuration Options

```go
type Config struct {
    // Required
    PayTo           string   // Wallet address to receive payments
    Network         string   // "base", "base-sepolia"
    Currency        string   // "USDC"
    Asset           string   // USDC contract address
    PricePerRequest int64    // Price in micro-units (6 decimals)
    FacilitatorURL  string   // Payment verification endpoint
    
    // Optional
    ExemptPaths     []string // Paths that don't require payment
    Description     string   // Description for 402 response
    MaxTimeout      int      // Payment validity in seconds (default: 30)
    
    // AI Agent Support
    AgentPricing    map[string]int64  // Custom pricing per agent
    BudgetHints     bool              // Include budget hints in response
}
```

## 🧪 Testing

```bash
# Run unit tests
make test

# Run E2E tests
./scripts/e2e-test.sh

# Test with curl
curl -v http://localhost:8402/api/data
# Returns 402 with payment requirements

curl -v http://localhost:8402/api/data \
  -H "X-PAYMENT: <signed-payment-header>"
# Returns 200 with data
```

## 🔗 Related Projects

- **[x402-hosted](https://github.com/siddimore/x-402-hosted)** - Multi-tenant hosted gateway with demo UI
- **[x402 Protocol](https://www.x402.org/)** - Official x402 specification
- **[Coinbase x402 SDK](https://github.com/coinbase/x402)** - Official JavaScript SDK

## 📄 License

MIT License - see [LICENSE](LICENSE)
