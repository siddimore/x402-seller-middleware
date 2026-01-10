// Example: Direct middleware integration
// This shows how to integrate x402 payment protection directly into your Go application
// without using the separate gateway binary.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/siddimore/x402-seller-middleware/pkg/x402"
)

// Article represents premium content
type Article struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// Sample data
var articles = []Article{
	{ID: "1", Title: "Introduction to Web3", Content: "Web3 represents the next evolution of the internet...", Author: "Alice", CreatedAt: time.Now()},
	{ID: "2", Title: "Understanding Blockchain", Content: "Blockchain is a distributed ledger technology...", Author: "Bob", CreatedAt: time.Now()},
	{ID: "3", Title: "Smart Contracts 101", Content: "Smart contracts are self-executing contracts...", Author: "Charlie", CreatedAt: time.Now()},
}

func main() {
	port := flag.String("port", "8080", "Server port")
	paymentURL := flag.String("payment-url", "https://pay.example.com/checkout", "Payment checkout URL")
	price := flag.Int64("price", 100, "Price per request in cents")
	flag.Parse()

	// Override with environment variables if set
	if env := os.Getenv("PORT"); env != "" {
		*port = env
	}
	if env := os.Getenv("PAYMENT_URL"); env != "" {
		*paymentURL = env
	}

	mux := http.NewServeMux()

	// ========================================
	// Public endpoints (no payment required)
	// ========================================

	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/api/preview/articles", handleArticlesList) // Free preview

	// ========================================
	// Protected endpoints (payment required)
	// ========================================

	mux.HandleFunc("/api/articles/", handleArticleDetail) // Full article content
	mux.HandleFunc("/api/premium/insights", handlePremiumInsights)

	// ========================================
	// Configure x402 middleware
	// ========================================

	config := x402.Config{
		PaymentEndpoint: *paymentURL,
		AcceptedMethods: []string{"Bearer", "Token", "X402"},
		PricePerRequest: *price,
		Currency:        "USD",
		ExemptPaths: []string{
			"/health",
			"/api/preview/", // Preview endpoints are free
		},
		// Optional: Custom payment verifier
		// PaymentVerifier: x402.NewHTTPVerifier(x402.VerifierConfig{
		// 	Endpoint: "https://your-payment-service.com/verify",
		// 	APIKey:   os.Getenv("PAYMENT_API_KEY"),
		// }),
	}

	// Wrap with x402 middleware
	handler := x402.Middleware(mux, config)

	// ========================================
	// Start server
	// ========================================

	addr := ":" + *port
	log.Println("📰 Premium Content API starting on", addr)
	log.Println("")
	log.Println("Free endpoints:")
	log.Println("  GET /health                 - Health check")
	log.Println("  GET /api/preview/articles   - Article list (preview)")
	log.Println("")
	log.Println("Paid endpoints (", *price, " cents per request):")
	log.Println("  GET /                       - API info")
	log.Println("  GET /api/articles/{id}      - Full article content")
	log.Println("  GET /api/premium/insights   - Premium analytics")
	log.Println("")
	log.Println("Test with:")
	log.Printf("  curl http://localhost:%s/api/articles/1           # Returns 402\n", *port)
	log.Printf("  curl -H 'Authorization: Bearer valid_token' http://localhost:%s/api/articles/1  # Works!\n", *port)

	log.Fatal(http.ListenAndServe(addr, handler))
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service":     "Premium Content API",
		"version":     "1.0.0",
		"description": "Example API protected by x402 payment middleware",
		"endpoints": map[string]string{
			"GET /":                     "This info (paid - use /health for free check)",
			"GET /health":               "Health check (free)",
			"GET /api/preview/articles": "List articles preview (free)",
			"GET /api/articles/{id}":    "Full article (paid)",
			"GET /api/premium/insights": "Premium insights (paid)",
		},
		"payment": map[string]interface{}{
			"price":    100,
			"currency": "USD",
			"methods":  []string{"Bearer token", "X-Payment-Token header", "payment_token query param"},
		},
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func handleArticlesList(w http.ResponseWriter, r *http.Request) {
	// Return preview only (titles, no full content)
	previews := make([]map[string]interface{}, len(articles))
	for i, a := range articles {
		previews[i] = map[string]interface{}{
			"id":         a.ID,
			"title":      a.Title,
			"author":     a.Author,
			"preview":    a.Content[:min(50, len(a.Content))] + "...",
			"created_at": a.CreatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"articles": previews,
		"message":  "Pay to access full article content",
	})
}

func handleArticleDetail(w http.ResponseWriter, r *http.Request) {
	// Extract article ID from path
	id := r.URL.Path[len("/api/articles/"):]
	if id == "" {
		http.Error(w, "Article ID required", http.StatusBadRequest)
		return
	}

	// Find article
	for _, a := range articles {
		if a.ID == id {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"article":          a,
				"payment_verified": r.Header.Get("X-Payment-Verified"),
				"message":          "🎉 Thank you for your payment! Enjoy the full content.",
			})
			return
		}
	}

	http.NotFound(w, r)
}

func handlePremiumInsights(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"insights": map[string]interface{}{
			"total_articles":     len(articles),
			"trending_topic":     "Web3",
			"reader_engagement":  "87%",
			"avg_read_time":      "4.2 minutes",
			"top_author":         "Alice",
			"revenue_this_month": "$12,450",
		},
		"message": "💎 Premium analytics unlocked!",
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
