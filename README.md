# X402 Seller Middleware for Go

A production-ready Go middleware implementation for HTTP 402 Payment Required protocol.

## 📁 Project Structure

```
seller-middleware/
├── cmd/
│   └── example/          # Example server
│       └── main.go
├── pkg/
│   └── x402/             # Public package
│       ├── doc.go        # Package documentation
│       ├── middleware.go # Core middleware
│       ├── middleware_test.go
│       └── verifier.go   # Payment verification utilities
├── internal/             # Internal utilities
│   └── utils.go
├── go.mod
├── README.md
├── Makefile
└── .gitignore
```

## Features

- 🔒 Intercepts requests and validates payment tokens
- 💰 Returns 402 Payment Required for unauthorized requests
- ⚙️ Configurable payment endpoints and pricing
- 🛣️ Supports exempt paths for public resources
- 🎫 Multiple authentication methods (Bearer, Token, custom headers)
- 🔧 Custom payment verifiers support

## Installation

```bash
go get github.com/siddimore/x402-seller-middleware/pkg/x402
```

## Usage

### Basic Example

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
| `Currency` | string | Currency code (e.g., "USD", "BTC") |
| `ExemptPaths` | []string | Paths that don't require payment |

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

## Running the Example

```bash
# Run the server
go run main.go

# Test public endpoint (no payment required)
curl http://localhost:8080/api/public

# Test protected endpoint without payment (returns 402)
curl -v http://localhost:8080/api/protected

# Test protected endpoint with valid payment token
curl -H "Authorization: Bearer valid_token123" \
  http://localhost:8080/api/protected
```

## Integration with Payment Providers

To integrate with a real payment provider, modify the `verifyPaymentToken` function in `middleware/x402.go`:

```go
func verifyPaymentToken(token string, config Config) (bool, error) {
    req, err := http.NewRequest("POST", config.PaymentEndpoint, nil)
    if err != nil {
        return false, err
    }
    
    req.Header.Set("Authorization", "Bearer "+token)
    
    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return false, err
    }
    defer resp.Body.Close()
    
    return resp.StatusCode == http.StatusOK, nil
}
```

## Response Headers

When payment is verified, the middleware adds these headers:

- `X-Payment-Verified: true`
- `X-Payment-Timestamp: <RFC3339 timestamp>`

When payment is required (402 response):

- `WWW-Authenticate: Bearer, Token realm="Payment Required"`
- `X-Payment-Required: true`
- `X-Payment-Amount: <amount>`
- `X-Payment-Currency: <currency>`

## License

MIT
