# Unified Payment Architecture

This document describes the unified payment architecture that supports **both crypto and fiat payment rails** in the x402 middleware.

## Overview

The x402 protocol is **payment-rail agnostic** - it defines HOW to negotiate payments over HTTP (402 responses, payment headers), not WHICH payment network to use. This means we can support:

- **Crypto rails**: EVM chains (Base, Ethereum), SVM chains (Solana)
- **Fiat rails**: Stripe (implemented), with architecture ready for future providers

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Client Request                               │
│   (AI Agent, Human User, HTTP Client)                               │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                  Unified Payment Middleware                         │
│                                                                     │
│  ┌───────────────────┐  ┌───────────────────┐  ┌─────────────────┐ │
│  │  Payment Proof    │  │  Rail Selection   │  │  Verification   │ │
│  │  Extraction       │  │  & Routing        │  │  & Capture      │ │
│  └───────────────────┘  └───────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
                                │
           ┌────────────────────┼────────────────────┐
           ▼                    ▼                    ▼
┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
│   Stripe Rail    │  │  EVM Crypto Rail │  │  Future Rails    │
│                  │  │                  │  │                  │
│  - Card payments │  │  - USDC on Base  │  │  - SVM (Solana)  │
│  - Apple Pay     │  │  - ETH transfers │  │  - ACH           │
│  - Google Pay    │  │  - EIP-3009      │  │  - Other fiat    │
└──────────────────┘  └──────────────────┘  └──────────────────┘
```

## Payment Rails

### PaymentRail Interface

Each payment rail implements the `PaymentRail` interface:

```go
type PaymentRail interface {
    ID() string
    DisplayName() string
    Type() RailType  // crypto, fiat, hybrid
    
    CreatePaymentIntent(ctx, req) (*PaymentIntent, error)
    VerifyPayment(ctx, req) (*PaymentVerification, error)
    CapturePayment(ctx, req) (*PaymentCapture, error)
    RefundPayment(ctx, req) (*PaymentRefund, error)
    
    WebhookHandler() http.Handler
}
```

### Supported Rails

| Rail | Type | Currencies | Status |
|------|------|------------|--------|
| `evm-crypto` | Crypto | USDC, ETH, WETH | ✅ Implemented |
| `stripe` | Fiat | USD, EUR, GBP, 135+ | ✅ Implemented |
| `svm-crypto` | Crypto | USDC (SPL) | 🚧 Planned |

## Usage

### Basic Setup

```go
config := x402.UnifiedPaymentConfig{
    // Pricing
    PricePerRequest: 100,   // $0.01 in cents
    Currency:        "USD",
    
    // Enable crypto
    CryptoEnabled:  true,
    CryptoPayTo:    "0x...",
    CryptoNetworks: []x402.NetworkType{x402.NetworkBaseMainnet},
    
    // Enable Stripe
    FiatEnabled:       true,
    StripeSecretKey:   os.Getenv("STRIPE_SECRET_KEY"),
    
    // Facilitator for crypto verification
    FacilitatorURL: "https://facilitator.x402.org",
}

// Create middleware
handler := x402.UnifiedPaymentMiddleware(yourHandler, config)
```

### AI Agent Support

```go
agentConfig := x402.AIAgentPaymentConfig{
    AllowCrypto:      true,
    AllowFiat:        true,
    MaxRequestBudget: 10000,  // $1.00 max per request
    PreAuthStore:     x402.NewInMemoryPreAuthStore(),
}

handler := x402.AIAgentPaymentMiddleware(yourHandler, config, agentConfig)
```

## Client Flow

### 1. Initial Request (No Payment)

```http
GET /api/premium/data HTTP/1.1
Host: api.example.com
```

### 2. Server Returns 402 with Options

```http
HTTP/1.1 402 Payment Required
Content-Type: application/json
PAYMENT-REQUIRED: eyJ4NDAyVmVyc2lvbiI6MSwib3B0aW9ucyI6Wy4uLl19

{
  "x402Version": 1,
  "options": [
    {
      "rail": "evm-crypto",
      "displayName": "Pay with Crypto (Base)",
      "type": "crypto",
      "scheme": "exact",
      "network": "eip155:8453",
      "amount": 100,
      "currency": "USD",
      "payTo": "0x...",
      "estimatedFee": 0
    },
    {
      "rail": "stripe",
      "displayName": "Pay with Card (Visa, Mastercard)",
      "type": "fiat",
      "amount": 100,
      "currency": "USD",
      "clientSecret": "pi_xxx_secret_xxx",
      "estimatedFee": 33
    }
  ],
  "accepts": [
    // Legacy x402 format for backwards compatibility
  ]
}
```

### 3a. Client Pays with Crypto

```http
GET /api/premium/data HTTP/1.1
Host: api.example.com
PAYMENT-SIGNATURE: {base64_encoded_payment_payload}
```

### 3b. Client Pays with Stripe

```http
GET /api/premium/data HTTP/1.1
Host: api.example.com
X-STRIPE-PAYMENT-INTENT: pi_xxx
```

Or via redirect:
```
GET /api/premium/data?payment_intent=pi_xxx
```

### 4. Server Returns Resource

```http
HTTP/1.1 200 OK
X-Payment-Verified: true
X-Payment-Rail: stripe
X-Payment-ID: pi_xxx

{"data": "premium content"}
```

## Customer Onboarding

For returning customers, you can save their preferred payment method:

```go
// Create onboarding handler
prefsStore := x402.NewInMemoryPaymentPrefsStore()
onboarding := x402.NewOnboardingHandler(config, prefsStore)

// Routes
mux.HandleFunc("/api/payment-methods", onboarding.ListPaymentMethods)
mux.HandleFunc("/api/preferences", onboarding.SetPreferredMethod)
mux.HandleFunc("/api/stripe/setup", onboarding.CreateStripeSetupIntent)
```

### Preference API

```bash
# List available payment methods
curl https://api.example.com/api/payment-methods

# Response
{
  "paymentMethods": [
    {"id": "stripe", "displayName": "Credit/Debit Card", "type": "fiat"},
    {"id": "evm-crypto", "displayName": "Cryptocurrency", "type": "crypto"}
  ]
}

# Set preferred method
curl -X POST https://api.example.com/api/preferences \
  -H "Content-Type: application/json" \
  -d '{"customerId": "cust_123", "rail": "stripe"}'
```

## AI Agent Integration

AI agents can pay using:

1. **Crypto** (x402 native): Sign payment with wallet
2. **Pre-authorized budget**: Pay from pre-funded balance
3. **Fiat** (via saved card): Use tokenized card on file

### Pre-Authorization Flow

```go
// Create pre-auth budget for agent
preAuthStore.Create(&x402.PreAuthBudget{
    AgentID:       "agent_123",
    TotalBudget:   100000,  // $10.00
    Currency:      "USD",
    ExpiresAt:     time.Now().Add(24 * time.Hour),
})

// Agent makes request with budget header
curl -X GET https://api.example.com/api/premium/data \
  -H "X-Agent-ID: agent_123" \
  -H "X-Agent-Budget: 10000"
```

## Adding New Payment Rails

The architecture is extensible. To add a new payment rail (e.g., ACH bank transfers):

```go
type ACHRail struct {
    APIKey string
}

func (a *ACHRail) ID() string { return "ach" }
func (a *ACHRail) DisplayName() string { return "Bank Transfer (ACH)" }
func (a *ACHRail) Type() RailType { return RailTypeFiat }

func (a *ACHRail) CreatePaymentIntent(ctx context.Context, req *PaymentIntentRequest) (*PaymentIntent, error) {
    // Call ACH provider API
}

// ... implement other methods

// Register the rail
registry := x402.NewRailRegistry()
registry.Register(&ACHRail{APIKey: os.Getenv("ACH_API_KEY")})
```

## Security Considerations

1. **Stripe webhook verification**: Always verify webhook signatures
2. **Crypto verification**: Use facilitator or verify signatures locally
3. **Pre-auth budgets**: Set expiration and limits
4. **CORS**: Expose `PAYMENT-REQUIRED` header for browser clients

## Future Roadmap

- [ ] Apple Pay / Google Pay via Stripe
- [ ] Solana (SVM) crypto rail
- [ ] ACH bank transfers
- [ ] Subscription/recurring payments
- [ ] Multi-currency support
