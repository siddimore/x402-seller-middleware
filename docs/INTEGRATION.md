# X402 Integration Guide

Protect any internet resource with HTTP 402 Payment Required using the x402-seller-middleware.

## 🎯 Integration Strategies (Least to Most Invasive)

### 1. **Reverse Proxy Gateway** (Zero code changes) ⭐ Recommended

Deploy the x402-gateway as a reverse proxy in front of your existing backend:

```
Client → x402-gateway → Your Backend
```

**Pros:** No code changes to your app, works with any backend language  
**Cons:** Additional hop, requires infrastructure setup

```bash
# Build and run
make gateway
./bin/x402-gateway \
  -backend=http://localhost:3000 \
  -payment-url=https://pay.example.com \
  -price=100 \
  -currency=USD
```

### 2. **Edge/CDN Integration** (Configuration only)

Deploy payment protection at the edge (Cloudflare, Vercel, AWS):

```
Client → CDN Edge (x402) → Origin
```

**Pros:** Global, fast, no origin changes  
**Cons:** Platform-specific, limited customization

See deployment examples in `/deploy/`:
- Cloudflare Workers: `deploy/cloudflare-worker/`
- Vercel Edge: `deploy/vercel-edge/`
- AWS Lambda@Edge: Use `deploy/universal/x402.js`

### 3. **Nginx/Load Balancer** (Infrastructure level)

Add x402 at your load balancer layer:

```
Client → Nginx (routes to x402-gateway) → Backend
```

**Pros:** Centralized, works with existing infrastructure  
**Cons:** Requires nginx/LB configuration

See: `deploy/nginx/nginx.conf`

### 4. **Middleware Integration** (Code-level)

Import and use x402 directly in your Go application:

```go
import "github.com/siddimore/x402-seller-middleware/pkg/x402"

handler := x402.Middleware(yourHandler, x402.Config{
    PaymentEndpoint: "https://pay.example.com",
    PricePerRequest: 100,
    Currency:        "USD",
    ExemptPaths:     []string{"/health", "/public"},
})
```

**Pros:** Most control, no extra services  
**Cons:** Requires code changes, Go-specific

---

## 🚀 Quick Start Examples

### Example 1: Protect an Express.js API (via Gateway)

```bash
# Terminal 1: Start your Express app
node your-express-app.js  # Running on port 3000

# Terminal 2: Start x402 gateway
./bin/x402-gateway -backend=http://localhost:3000 -listen=:8402

# Test it
curl http://localhost:8402/api/data          # Returns 402
curl -H "Authorization: Bearer valid_token" http://localhost:8402/api/data  # Works!
```

### Example 2: Protect a Python FastAPI (via Docker)

```yaml
# docker-compose.yml
services:
  api:
    build: ./your-python-app
    ports:
      - "3000:3000"
  
  x402:
    image: x402-gateway:latest
    environment:
      - X402_BACKEND_URL=http://api:3000
      - X402_PAYMENT_ENDPOINT=https://pay.example.com
    ports:
      - "8402:8402"
```

### Example 3: Cloudflare Worker (Edge)

```bash
cd deploy/cloudflare-worker
npm install
npx wrangler dev  # Test locally

# Deploy
npx wrangler deploy
```

---

## 🔑 Payment Token Flow

```
1. Client requests protected resource
   GET /api/data
   
2. Server returns 402 Payment Required
   {
     "status": 402,
     "amount": 100,
     "currency": "USD",
     "payment_url": "https://pay.example.com/checkout"
   }

3. Client makes payment at payment_url
   → Receives payment token

4. Client retries with token
   GET /api/data
   Authorization: Bearer <payment_token>

5. Server verifies token and grants access
   X-Payment-Verified: true
```

## 📋 Token Verification Options

### Option A: Static Token List (Simple)
```go
config := x402.Config{
    PaymentVerifier: x402.NewStaticVerifier([]string{
        "token_abc123",
        "token_xyz789",
    }),
}
```

### Option B: HTTP Verification Endpoint
```go
config := x402.Config{
    PaymentVerifier: x402.NewHTTPVerifier(x402.VerifierConfig{
        Endpoint: "https://your-payment-service.com/verify",
        APIKey:   "your-api-key",
    }),
}
```

### Option C: JWT Tokens
```go
config := x402.Config{
    PaymentVerifier: x402.NewJWTVerifier("your-secret-key"),
}
```

### Option D: Custom Verification
```go
config := x402.Config{
    PaymentVerifier: func(token string) (bool, error) {
        // Your custom logic
        return verifyWithYourSystem(token)
    },
}
```

---

## 🌐 CDN/Edge Platform Guides

### Cloudflare Workers

1. Copy `deploy/cloudflare-worker/` to your project
2. Configure `wrangler.toml` with your settings
3. Set secrets: `wrangler secret put VALID_TOKENS`
4. Deploy: `wrangler deploy`

### Vercel Edge

1. Copy `deploy/vercel-edge/middleware.ts` to project root
2. Set environment variables in Vercel dashboard
3. Deploy normally - middleware runs automatically

### AWS CloudFront + Lambda@Edge

1. Use `deploy/universal/x402.js` with `x402LambdaEdgeHandler`
2. Create Lambda@Edge function (viewer-request trigger)
3. Associate with CloudFront distribution

### Fastly Compute@Edge

Use the `deploy/universal/x402.js` with Fastly's JS runtime.

---

## 🛡️ Security Best Practices

1. **Always use HTTPS** - Tokens can be intercepted on HTTP
2. **Set token expiration** - Tokens should expire after use or time
3. **Rate limit** - Prevent brute-force token guessing
4. **Use external verification** - Don't rely on static tokens in production
5. **Audit logging** - Log all payment verification attempts

---

## 📊 Headers Reference

### Request Headers (Client → Server)
| Header | Description |
|--------|-------------|
| `Authorization: Bearer <token>` | Standard bearer token |
| `Authorization: X402 <token>` | X402-specific scheme |
| `X-Payment-Token: <token>` | Custom header |
| `X-402-Token: <token>` | Standardized X402 header |

### Response Headers (402 Response)
| Header | Description |
|--------|-------------|
| `X-Payment-Required: true` | Indicates payment needed |
| `X-Payment-Amount: 100` | Required payment amount |
| `X-Payment-Currency: USD` | Currency code |
| `X-Payment-URL: <url>` | Where to make payment |
| `WWW-Authenticate: Bearer realm="..."` | Auth challenge |

### Response Headers (Success)
| Header | Description |
|--------|-------------|
| `X-Payment-Verified: true` | Payment was verified |
| `X-Payment-Timestamp: <ISO8601>` | When verified |

---

## 🔧 Configuration Reference

| Option | Env Var | Default | Description |
|--------|---------|---------|-------------|
| `--listen` | `X402_LISTEN_ADDR` | `:8402` | Gateway listen address |
| `--backend` | `X402_BACKEND_URL` | - | Backend URL to proxy |
| `--payment-url` | `X402_PAYMENT_ENDPOINT` | - | Payment checkout URL |
| `--price` | `X402_PRICE` | `100` | Price per request |
| `--currency` | `X402_CURRENCY` | `USD` | Currency code |
| `--exempt` | `X402_EXEMPT_PATHS` | `/health` | Exempt paths (comma-sep) |

---

## 🤝 Contributing

We welcome contributions! See the main README for guidelines.

## 📄 License

MIT License - see LICENSE file.
