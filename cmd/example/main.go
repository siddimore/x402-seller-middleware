// Example server demonstrating the x402 seller middleware
package main

import (
	"log"
	"net/http"

	"github.com/siddimore/x402-seller-middleware/pkg/x402"
)

func main() {
	// Create a new HTTP mux
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "healthy"}`))
	})

	// Protected endpoint that requires payment
	mux.HandleFunc("/api/protected", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "Access granted to protected resource"}`))
	})

	// Another protected endpoint
	mux.HandleFunc("/api/premium", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "Premium content accessed"}`))
	})

	// Public endpoint (exempt from payment)
	mux.HandleFunc("/api/public", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "This is a public endpoint"}`))
	})

	// Configure the X402 middleware
	config := x402.Config{
		PaymentEndpoint: "https://payment-provider.example.com/verify",
		AcceptedMethods: []string{"Bearer", "Token"},
		PricePerRequest: 100, // Price in smallest currency unit (e.g., cents)
		Currency:        "USD",
		ExemptPaths:     []string{"/api/public", "/health"},
	}

	// Wrap the mux with the X402 seller middleware
	handler := x402.Middleware(mux, config)

	log.Println("🚀 X402 Example Server starting on :8080")
	log.Println("📖 Endpoints:")
	log.Println("   GET /health      - Health check (free)")
	log.Println("   GET /api/public  - Public endpoint (free)")
	log.Println("   GET /api/protected - Protected (requires payment)")
	log.Println("   GET /api/premium   - Premium (requires payment)")
	log.Println("")
	log.Println("💡 Test with:")
	log.Println("   curl http://localhost:8080/api/protected")
	log.Println("   curl -H 'Authorization: Bearer valid_token' http://localhost:8080/api/protected")

	log.Fatal(http.ListenAndServe(":8080", handler))
}
