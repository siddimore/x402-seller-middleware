// Example: Unified Payment Server
// This example demonstrates a server that accepts both crypto and fiat payments
// for AI agents, human users, and HTTP clients.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/siddimore/x402-seller-middleware/pkg/x402"
)

// Ensure imports are used
var _ = context.Background
var _ = time.Now

func main() {
	// =====================================
	// Configure unified payment middleware
	// =====================================
	config := x402.UnifiedPaymentConfig{
		// Pricing
		PricePerRequest: 100,   // $0.01 in cents (or 100 USDC units)
		Currency:        "USD", // Primary currency

		// Crypto settings
		CryptoEnabled: true,
		CryptoPayTo:   os.Getenv("CRYPTO_PAY_TO"), // Your wallet address
		CryptoAsset:   os.Getenv("CRYPTO_ASSET"),  // USDC contract
		CryptoScheme:  "exact",
		CryptoNetworks: []x402.NetworkType{
			x402.NetworkBaseMainnet,
			x402.NetworkBaseSepolia, // Testnet
		},

		// Fiat settings (Stripe)
		FiatEnabled:         true,
		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),

		// Facilitator for crypto verification
		FacilitatorURL: os.Getenv("FACILITATOR_URL"),

		// Callbacks
		OnPaymentSuccess: func(ctx context.Context, payment *x402.CompletedPayment) {
			log.Printf("Payment successful: %s via %s ($%.2f)",
				payment.ID,
				payment.Rail,
				float64(payment.Amount)/100,
			)
		},

		// Exempt paths that don't require payment
		ExemptPaths: []string{
			"/health",
			"/",
			"/api/payment-methods",
			"/api/onboarding/",
			"/stripe/webhook",
		},
	}

	// AI agent-specific config
	agentConfig := x402.AIAgentPaymentConfig{
		AllowCrypto:      true,
		AllowFiat:        true,
		MaxRequestBudget: 10000,   // $1.00 max per request
		MaxSessionBudget: 100000,  // $10.00 max per session
		MaxDailyBudget:   1000000, // $100.00 max per day
		PreAuthStore:     x402.NewInMemoryPreAuthStore(),
	}

	// Create onboarding handler for payment method setup
	prefsStore := x402.NewInMemoryPaymentPrefsStore()
	onboarding := x402.NewOnboardingHandler(config, prefsStore)

	// =====================================
	// Set up routes
	// =====================================
	mux := http.NewServeMux()

	// Health check (exempt from payment)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	// Landing page (exempt from payment)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, landingPageHTML)
	})

	// =====================================
	// Payment onboarding endpoints (exempt)
	// =====================================

	// List available payment methods
	mux.HandleFunc("/api/payment-methods", onboarding.ListPaymentMethods)

	// Set customer's preferred payment method
	mux.HandleFunc("/api/onboarding/preferences", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			onboarding.GetPreferences(w, r)
		case "POST":
			onboarding.SetPreferredMethod(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Create Stripe setup intent for saving card
	mux.HandleFunc("/api/onboarding/stripe/setup", onboarding.CreateStripeSetupIntent)

	// Stripe webhook handler
	if config.FiatEnabled && config.StripeSecretKey != "" {
		stripeRail := x402.NewStripeRail(config.StripeSecretKey, config.StripeWebhookSecret)
		mux.Handle("/stripe/webhook", stripeRail.WebhookHandler())
	}

	// =====================================
	// Protected API endpoints (require payment)
	// =====================================

	// Premium API endpoint
	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("/api/premium/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "This is premium data!",
			"data": map[string]interface{}{
				"timestamp": time.Now().Unix(),
				"value":     42,
			},
		})
	})

	protectedMux.HandleFunc("/api/premium/ai-analysis", func(w http.ResponseWriter, r *http.Request) {
		// This endpoint is specifically designed for AI agents
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"analysis":    "AI-generated insights...",
			"confidence":  0.95,
			"tokens_used": 150,
		})
	})

	// Wrap protected routes with unified payment middleware
	// This supports both crypto AND fiat payments
	protectedHandler := x402.AIAgentPaymentMiddleware(
		protectedMux,
		config,
		agentConfig,
	)

	// Mount protected routes
	mux.Handle("/api/premium/", protectedHandler)

	// =====================================
	// Start server
	// =====================================
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting unified payment server on :%s", port)
	log.Printf("Crypto payments: %v (Networks: %v)", config.CryptoEnabled, config.CryptoNetworks)
	log.Printf("Fiat payments: %v (Stripe)", config.FiatEnabled)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// Landing page HTML
var landingPageHTML = `<!DOCTYPE html>
<html>
<head>
    <title>Unified Payment API</title>
    <script src="https://js.stripe.com/v3/"></script>
    <style>
        body { font-family: system-ui, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
        .card { border: 1px solid #ddd; padding: 20px; margin: 20px 0; border-radius: 8px; }
        .btn { padding: 12px 24px; cursor: pointer; border: none; border-radius: 6px; font-size: 16px; }
        .btn-crypto { background: #6366f1; color: white; }
        .btn-fiat { background: #10b981; color: white; }
        .payment-options { display: flex; gap: 20px; margin-top: 20px; }
        h1 { color: #1f2937; }
        h2 { color: #374151; }
        code { background: #f3f4f6; padding: 2px 6px; border-radius: 4px; }
    </style>
</head>
<body>
    <h1>🔐 Unified Payment API</h1>
    <p>This API accepts payments via <strong>Crypto (USDC on Base)</strong> OR <strong>Credit Card (Stripe)</strong>.</p>

    <div class="card">
        <h2>Choose Your Payment Method</h2>
        <div class="payment-options">
            <button class="btn btn-crypto" onclick="payCrypto()">
                💎 Pay with Crypto
            </button>
            <button class="btn btn-fiat" onclick="payWithCard()">
                💳 Pay with Card
            </button>
        </div>
    </div>

    <div class="card">
        <h2>For AI Agents</h2>
        <p>AI agents can pay using either method:</p>
        <pre>
// Crypto payment (x402 protocol)
curl -X GET https://api.example.com/api/premium/data \
  -H "PAYMENT-SIGNATURE: {base64_payment_payload}"

// Or with pre-authorized budget
curl -X GET https://api.example.com/api/premium/data \
  -H "X-Agent-ID: agent_123" \
  -H "X-Agent-Budget: 10000"
        </pre>
    </div>

    <div class="card">
        <h2>API Endpoints</h2>
        <ul>
            <li><code>GET /api/payment-methods</code> - List available payment methods</li>
            <li><code>POST /api/onboarding/preferences</code> - Set preferred payment method</li>
            <li><code>GET /api/premium/data</code> - Premium data (requires payment)</li>
            <li><code>GET /api/premium/ai-analysis</code> - AI analysis (requires payment)</li>
        </ul>
    </div>

    <script>
        async function payCrypto() {
            // Fetch payment requirements
            const response = await fetch('/api/premium/data');
            if (response.status === 402) {
                const data = await response.json();
                console.log('Payment required:', data);
                // In production: use x402 client to create and sign payment
                alert('Crypto payment: Use x402 client SDK to complete payment');
            }
        }

        async function payWithCard() {
            // Fetch payment options
            const response = await fetch('/api/premium/data');
            if (response.status === 402) {
                const data = await response.json();
                const stripeOption = data.options?.find(o => o.rail === 'stripe');
                if (stripeOption) {
                    // In production: use Stripe.js to collect card and confirm payment
                    alert('Card payment: Use Stripe.js with clientSecret: ' + stripeOption.clientSecret);
                }
            }
        }
    </script>
</body>
</html>`
