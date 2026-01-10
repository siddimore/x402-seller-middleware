// Simple test backend server to verify x402 gateway functionality
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

func main() {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "healthy",
			"server": "test-backend",
		})
	})

	// Public endpoint (should be exempt from payment)
	mux.HandleFunc("/api/public", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":   "This is a public endpoint - no payment required!",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// Protected endpoint - this is what we want to protect with x402
	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if request came through x402 gateway (will have this header if payment verified)
		paymentVerified := r.Header.Get("X-Payment-Verified")

		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":          "🎉 You accessed protected data!",
			"payment_verified": paymentVerified,
			"timestamp":        time.Now().Format(time.RFC3339),
			"headers_received": getRelevantHeaders(r),
		})
	})

	// Premium content endpoint
	mux.HandleFunc("/api/premium", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "💎 Premium content unlocked!",
			"data": map[string]interface{}{
				"secret":    "The answer is 42",
				"premium":   true,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		})
	})

	// Echo endpoint - useful for debugging
	mux.HandleFunc("/api/echo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"method":  r.Method,
			"path":    r.URL.Path,
			"query":   r.URL.Query(),
			"headers": getRelevantHeaders(r),
		})
	})

	// Root handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "Test Backend Server",
			"endpoints": []string{
				"GET /health     - Health check",
				"GET /api/public - Public (free) endpoint",
				"GET /api/data   - Protected endpoint",
				"GET /api/premium - Premium content",
				"GET /api/echo   - Echo request details",
			},
		})
	})

	port := ":3000"
	log.Println("🖥️  Test Backend Server starting on", port)
	log.Println("")
	log.Println("Endpoints:")
	log.Println("  GET http://localhost:3000/health      - Health check")
	log.Println("  GET http://localhost:3000/api/public  - Public endpoint")
	log.Println("  GET http://localhost:3000/api/data    - Protected data")
	log.Println("  GET http://localhost:3000/api/premium - Premium content")
	log.Println("  GET http://localhost:3000/api/echo    - Echo headers")
	log.Println("")
	log.Println("This server should be accessed through the x402 gateway!")

	log.Fatal(http.ListenAndServe(port, mux))
}

// getRelevantHeaders extracts headers useful for debugging
func getRelevantHeaders(r *http.Request) map[string]string {
	relevant := map[string]string{}
	keys := []string{
		"Authorization",
		"X-Payment-Token",
		"X-Payment-Verified",
		"X-Payment-Timestamp",
		"X-Forwarded-Host",
		"X-Forwarded-For",
		"X-Real-IP",
	}
	for _, key := range keys {
		if val := r.Header.Get(key); val != "" {
			relevant[key] = val
		}
	}
	return relevant
}
